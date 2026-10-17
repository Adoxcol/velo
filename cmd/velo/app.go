package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"velo/internal/engine"
	"velo/internal/i18n"
	"velo/internal/ipc"
	"velo/internal/version"
)

type item struct {
	d        *engine.Download
	cancel   context.CancelFunc // set while running
	running  bool
	requeue  bool // stopped by the schedule: go back to the queue, not "paused"
	lastDone int64
	speed    float64   // bytes/s, updated every second
	added    time.Time // when it was added
	finished time.Time // when it completed (zero until then)

	// Network drops: the download waits in the queue until retryAt.
	retryAt   time.Time
	retries   int   // in a row without progress
	retryDone int64 // bytes done at the last failure
}

// retryDelay is how long to wait before the nth retry in a row.
var retryDelay = func(n int) time.Duration {
	return min(5*time.Second<<(n-1), time.Minute)
}

const maxRetries = 30 // in a row without progress: about half an hour

type app struct {
	eng       *engine.Engine
	token     string
	mu        sync.Mutex
	items     []*item
	nextID    int
	settings  Settings
	running   int
	finished  bool          // a download completed since the last finish action
	wake      chan struct{} // nudges the dispatcher
	onDone    func(d *engine.Download)
	onFail    func(d *engine.Download, err error)
	show      func() // opens the main window
	quit      func() // exits the app
	showBatch func() // opens the window's batch dialog
	changed   func() // settings changed (updates the tray menu)
	onTick    func() // every second (taskbar progress)

	lastHistory   []byte      // last saved history.json contents
	speedHist     []float64   // total speed per second, last minute
	extSeen       time.Time   // last message from the browser extension
	batch         *batchReq   // links waiting in the batch dialog
	miniDismissed bool        // Velo Mini closed by the user until downloads stop
	upd           updater     // GitHub Releases updates
	pending       []pendingDL // browser downloads waiting for the download dialog
	nextPending   int
	showPending   func() // opens the download dialog for pending downloads
	defaultDir    string // download folder when the setting is empty
	home          string // data folder (%APPDATA%\Velo), fixed at start
}

func newApp(eng *engine.Engine) *app {
	a := &app{eng: eng, token: ipc.NewToken(), wake: make(chan struct{}, 1), settings: loadSettings(), defaultDir: eng.Dir(), home: ipc.Dir()}
	eng.Limit.SetRate(int64(a.settings.SpeedLimitKB) * 1024)
	if a.settings.DownloadDir != "" {
		eng.SetDir(a.settings.DownloadDir)
	}
	eng.SetSortByType(a.settings.SortByType)
	eng.SetProxy(a.settings.Proxy)
	if a.settings.Connections > 0 {
		eng.SetSegments(a.settings.Connections)
	}
	a.settings.Autostart = autostartEnabled() // the registry is the truth
	a.restore()
	go a.measure()
	go a.dispatcher()
	return a
}

func (a *app) get(id int) *item {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, it := range a.items {
		if it.d.ID == id {
			return it
		}
	}
	return nil
}

func (a *app) register(d *engine.Download) *item {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextID++
	d.ID = a.nextID
	it := &item{d: d, lastDone: d.Done.Load(), added: time.Now()}
	a.items = append(a.items, it)
	return it
}

// batchReq is a list of links sent by "Download all with Velo".
type batchReq struct {
	Page      string `json:"page"`
	Referrer  string `json:"referrer"`
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
	Links     []struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	} `json:"links"`
}

// batchLink is one row in the batch dialog.
type batchLink struct {
	URL      string `json:"url"`
	Text     string `json:"text"`
	Name     string `json:"name"`
	Category string `json:"category"` // "" for pages and unknown types
	File     bool   `json:"file"`     // a downloadable file type (pre-selected)
}

type batchView struct {
	Page  string      `json:"page"`
	Links []batchLink `json:"links"`
}

// takeBatch hands the pending batch to the window (once), labelling each
// link with its file name and type.
func (a *app) takeBatch() *batchView {
	a.mu.Lock()
	b := a.batch
	a.mu.Unlock()
	if b == nil {
		return nil
	}
	v := &batchView{Page: b.Page}
	for _, l := range b.Links {
		ok, kind := engine.Downloadable(l.URL)
		name := linkName(l.URL)
		cat := engine.CategoryOf(name)
		if kind != "" {
			cat = "Video"
		}
		v.Links = append(v.Links, batchLink{URL: l.URL, Text: l.Text, Name: name, Category: cat, File: ok})
	}
	return v
}

// finishBatch queues the chosen links and clears the pending batch.
func (a *app) finishBatch(urls []string, start bool) int {
	a.mu.Lock()
	b := a.batch
	a.batch = nil
	a.mu.Unlock()
	if b == nil {
		return 0
	}
	// Only links that were in the batch can be added.
	allowed := map[string]bool{}
	for _, l := range b.Links {
		allowed[l.URL] = true
	}
	var chosen []string
	for _, u := range urls {
		if allowed[u] {
			chosen = append(chosen, u)
		}
	}
	return a.addBatch(*b, chosen, start)
}

// addBatch queues the chosen links from the batch dialog.
func (a *app) addBatch(b batchReq, urls []string, start bool) int {
	n := 0
	for _, u := range urls {
		ok, kind := engine.Downloadable(u)
		if !ok && !strings.HasPrefix(strings.ToLower(u), "http") {
			continue
		}
		r := engine.Request{URL: u, Referrer: b.Referrer, UserAgent: b.UserAgent, Kind: kind}
		if sameHost(u, b.Page) { // never send the page's cookies to other sites
			r.Cookies = b.Cookies
		}
		if start {
			a.add(r)
		} else {
			a.register(&engine.Download{Req: r, Status: "paused"})
		}
		n++
	}
	a.kick()
	return n
}

func sameHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && ua.Host != "" && strings.EqualFold(ua.Host, ub.Host)
}

// extensionSeen is when the browser extension last talked to Velo (unix ms, 0 = never this session).
func (a *app) extensionSeen() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.extSeen.IsZero() {
		return 0
	}
	return a.extSeen.UnixMilli()
}

// pendingDL is a browser download waiting for the user in the download dialog.
type pendingDL struct {
	ID   int            `json:"id"`
	Req  engine.Request `json:"-"`
	Size int64          `json:"size"`
}

// pendingView is what the dialog shows for a pending download.
type pendingView struct {
	ID       int    `json:"id"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Site     string `json:"site"`
	Kind     string `json:"kind"`
}

// clearPending drops every browser download waiting for the dialog.
func (a *app) clearPending() {
	a.mu.Lock()
	a.pending = nil
	a.mu.Unlock()
}

// nextPendingDL returns the oldest pending browser download, or nil.
func (a *app) nextPendingDL() *pendingView {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending) == 0 {
		return nil
	}
	p := a.pending[0]
	v := &pendingView{ID: p.ID, URL: p.Req.URL, Filename: p.Req.Filename, Size: p.Size, Kind: p.Req.Kind}
	if u, err := url.Parse(p.Req.Referrer); err == nil && u.Host != "" {
		v.Site = u.Host
	} else if u, err := url.Parse(p.Req.URL); err == nil {
		v.Site = u.Host
	}
	return v
}

// takePending removes a pending download and returns its request.
func (a *app) takePending(id int) (engine.Request, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, p := range a.pending {
		if p.ID == id {
			a.pending = append(a.pending[:i], a.pending[i+1:]...)
			return p.Req, true
		}
	}
	return engine.Request{}, false
}

// addOptions come from the Add Download dialog.
type addOptions struct {
	PendingID int    `json:"pendingId"` // a browser download being confirmed
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	Dir       string `json:"dir"`      // "" = default folder
	Segments  int    `json:"segments"` // 0 = default
	Start     bool   `json:"start"`    // false = add paused
	Checksum  string `json:"checksum"` // optional, verified when done
	Queue     string `json:"queue"`    // named queue ID, "" = main
	User      string `json:"user"`     // optional HTTP login
	Pass      string `json:"pass"`
}

func (a *app) addWith(o addOptions) (int, error) {
	sum := ""
	if strings.TrimSpace(o.Checksum) != "" {
		var err error
		if sum, err = engine.ParseChecksum(o.Checksum); err != nil {
			return 0, err
		}
	}
	if o.PendingID != 0 {
		base, ok := a.takePending(o.PendingID)
		if !ok {
			return 0, errors.New("that download was already handled")
		}
		// Keep the browser's cookies, referrer and user agent.
		if name := strings.TrimSpace(o.Filename); name != "" {
			base.Filename = name // otherwise keep the browser's suggested name
		}
		base.Dir, base.Segments, base.Checksum, base.Queue = o.Dir, o.Segments, sum, o.Queue
		if o.User != "" {
			base.User, base.Pass = o.User, o.Pass
		}
		if o.Dir != "" && !filepath.IsAbs(o.Dir) {
			return 0, errors.New("choose a full folder path")
		}
		if !o.Start {
			it := a.register(&engine.Download{Req: base, Status: "paused"})
			a.kick()
			return it.d.ID, nil
		}
		return a.add(base), nil
	}
	ok, kind := engine.Downloadable(o.URL)
	u := strings.ToLower(o.URL)
	if !ok && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return 0, errors.New("enter a link starting with http:// or https://")
	}
	if o.Dir != "" && !filepath.IsAbs(o.Dir) {
		return 0, errors.New("choose a full folder path")
	}
	r := engine.Request{URL: o.URL, Filename: strings.TrimSpace(o.Filename), Dir: o.Dir, Segments: o.Segments, Kind: kind, Checksum: sum, User: o.User, Pass: o.Pass, Queue: o.Queue}
	if !o.Start {
		it := a.register(&engine.Download{Req: r, Status: "paused"})
		a.kick()
		return it.d.ID, nil
	}
	return a.add(r), nil
}

func (a *app) add(r engine.Request) int {
	it := a.register(&engine.Download{Req: r})
	log.Printf("#%d queued %s", it.d.ID, r.URL)
	a.enqueue(it)
	return it.d.ID
}

func (a *app) enqueue(it *item) {
	a.mu.Lock()
	it.retryAt, it.retries = time.Time{}, 0
	a.mu.Unlock()
	it.d.SetStatus("queued")
	a.kick()
}

func (a *app) kick() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// dispatcher starts queued downloads while slots are free and the schedule
// allows, stops running ones when the schedule window closes, and runs the
// "when finished" action. It re-checks every 15s for schedule changes.
func (a *app) dispatcher() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		a.dispatch(time.Now())
		select {
		case <-a.wake:
		case <-t.C:
		}
	}
}

// queueOf returns the schedule and own limit (0 = none) of a download's
// queue. Downloads in a queue that no longer exists use the main queue.
func (a *app) queueOf(it *item) (Schedule, int, string) {
	if id := it.d.QueueID(); id != "" {
		for _, q := range a.settings.Queues {
			if q.ID == id {
				return q.Schedule, q.MaxConcurrent, id
			}
		}
	}
	return a.settings.Schedule, 0, ""
}

func (a *app) dispatch(now time.Time) {
	a.mu.Lock()
	perQueue := map[string]int{} // running downloads in each queue
	for _, it := range a.items {
		if it.running {
			_, _, q := a.queueOf(it)
			perQueue[q]++
		}
	}
	queued := 0
	for _, it := range a.items {
		sched, limit, q := a.queueOf(it)
		allowed := sched.allowed(now)
		if it.running && !allowed && !it.requeue {
			it.requeue = true
			it.cancel()
			log.Printf("#%d stopped by schedule", it.d.ID)
		}
		if s, _ := it.d.State(); s == "queued" && !it.running && !now.Before(it.retryAt) {
			if allowed && a.running < a.settings.MaxConcurrent && (limit == 0 || perQueue[q] < limit) {
				a.launch(it)
				perQueue[q]++
			} else {
				queued++
			}
		}
	}
	idle := a.running == 0 && queued == 0
	action := ""
	if idle && a.finished {
		a.finished = false
		action = a.settings.OnFinish
	}
	a.mu.Unlock()
	a.saveHistory()
	if action != "" && action != "none" {
		a.finishAction(action)
	}
}

// launch starts it; a.mu must be held.
func (a *app) launch(it *item) {
	ctx, cancel := context.WithCancel(context.Background())
	it.cancel, it.running, it.requeue = cancel, true, false
	a.running++
	d := it.d
	go func() {
		defer cancel()
		t := time.Now()
		err := a.eng.Run(ctx, d)
		a.mu.Lock()
		it.running = false
		a.running--
		requeue := it.requeue
		if err == nil {
			a.finished = true
			it.retries = 0
		}
		retry := time.Duration(0)
		if engine.Transient(err) {
			if done := d.Done.Load(); done > it.retryDone {
				it.retries = 0 // it got further: the connection works sometimes
				it.retryDone = done
			}
			if it.retries < maxRetries {
				it.retries++
				retry = retryDelay(it.retries)
				it.retryAt = time.Now().Add(retry)
			}
		}
		a.mu.Unlock()
		switch {
		case retry > 0:
			log.Printf("#%d interrupted (%v); retrying in %s", d.ID, err, retry)
			d.SetStatus("queued")
			time.AfterFunc(retry, a.kick)
		case requeue && errors.Is(err, context.Canceled):
			d.SetStatus("queued") // resumes when the schedule allows
		case errors.Is(err, context.Canceled):
			log.Printf("#%d paused at %.1f%%", d.ID, pct(d))
		case err != nil:
			log.Printf("#%d failed: %v", d.ID, err)
			if a.onFail != nil {
				a.onFail(d, err)
			}
		default:
			a.mu.Lock()
			it.finished = time.Now()
			a.mu.Unlock()
			p, _ := d.Info()
			log.Printf("#%d done %s (%.1f MB in %.1fs)", d.ID, p, float64(d.Done.Load())/1e6, time.Since(t).Seconds())
			if a.onDone != nil {
				a.onDone(d)
			}
		}
		a.kick()
	}()
}

func (a *app) finishAction(action string) {
	log.Printf("all downloads finished: %s", action)
	switch action {
	case "exit":
		if a.quit != nil {
			a.quit()
		}
	case "sleep":
		exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Start()
	case "shutdown":
		// 60s grace period; "shutdown /a" cancels it.
		exec.Command("shutdown", "/s", "/t", "60", "/c",
			"Velo finished all downloads. Windows will shut down in 1 minute. Run 'shutdown /a' to cancel.").Start()
	}
}

func (a *app) pause(id int) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if it.running {
		it.requeue = false
		it.cancel()
		return nil // the dispatcher saves once the engine stops
	}
	s, _ := it.d.State()
	if s != "queued" {
		return errors.New("download is " + s)
	}
	it.d.SetStatus("paused")
	a.kick()
	return nil
}

// setQueue moves a download to a named queue ("" = the main queue).
func (a *app) setQueue(id int, queue string) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	if queue != "" {
		found := false
		for _, q := range a.getSettings().Queues {
			found = found || q.ID == queue
		}
		if !found {
			return errors.New("no such queue")
		}
	}
	it.d.SetQueue(queue)
	a.kick()
	return nil
}

// reorder moves a waiting download in the queue: "next" to the front,
// "up"/"down" past its neighbour in the queue, "now" starts it at once even
// when the downloads-at-once limit is reached.
func (a *app) reorder(id int, how string) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	if s, _ := it.d.State(); s != "queued" && s != "paused" && s != "error" {
		return errors.New("only waiting downloads can be moved")
	}
	a.mu.Lock()
	idx := -1
	var waiting []int // indexes of items in the queue, in order
	for i, x := range a.items {
		if x == it {
			idx = i
		}
		if s, _ := x.d.State(); !x.running && (s == "queued" || s == "paused" || s == "error") {
			waiting = append(waiting, i)
		}
	}
	pos := -1
	for k, i := range waiting {
		if i == idx {
			pos = k
		}
	}
	move := func(to int) { // to an index in a.items
		x := a.items[idx]
		a.items = append(a.items[:idx], a.items[idx+1:]...)
		a.items = append(a.items[:to], append([]*item{x}, a.items[to:]...)...)
	}
	switch how {
	case "next", "now":
		if pos > 0 {
			move(waiting[0])
		}
	case "up":
		if pos > 0 {
			move(waiting[pos-1])
		}
	case "down":
		if pos >= 0 && pos < len(waiting)-1 {
			move(waiting[pos+1])
		}
	default:
		a.mu.Unlock()
		return fmt.Errorf("unknown move %q", how)
	}
	start := how == "now"
	if start {
		it.retryAt, it.retries = time.Time{}, 0
		it.d.SetStatus("queued")
		a.launch(it) // past the limit: the user asked for this one now
	}
	a.mu.Unlock()
	if how == "next" {
		if s, _ := it.d.State(); s != "queued" {
			a.enqueue(it)
		}
	}
	a.kick()
	return nil
}

// refreshLink swaps an expired link for a fresh one and resumes the same
// file, like IDM's "refresh download address". The new link must serve a
// file of the same size, so two different files are never stitched together.
func (a *app) refreshLink(id int, newURL string) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	newURL = strings.TrimSpace(newURL)
	if u := strings.ToLower(newURL); !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("enter a link starting with http:// or https://")
	}
	a.mu.Lock()
	running := it.running
	a.mu.Unlock()
	d := it.d
	if running {
		return errors.New("pause the download first")
	}
	if s, _ := d.State(); s == "done" {
		return errors.New("this download has already finished")
	}
	if d.Req.Kind != "" {
		return errors.New("video streams can't change their link; add the new one as a new download")
	}
	req := d.Req
	if hostOf(req.URL) != hostOf(newURL) {
		req.Cookies = "" // the old site's login isn't for the new one
	}
	req.URL = newURL
	_, total := d.Info()
	if total > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		info, err := a.eng.Probe(ctx, req)
		cancel()
		if err != nil {
			return fmt.Errorf("couldn't open the new link: %w", err)
		}
		if info.Size != total {
			return fmt.Errorf("the new link is a different file (%d bytes, this download is %d bytes)", info.Size, total)
		}
		if !info.Resumable {
			return errors.New("the new link can't continue a download (the server doesn't support resuming)")
		}
	}
	d.SetRequest(req)
	log.Printf("#%d link refreshed", d.ID)
	a.enqueue(it)
	return nil
}

func hostOf(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func (a *app) resume(id int) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	if s, _ := it.d.State(); s != "paused" && s != "error" {
		return errors.New("download is " + s)
	}
	a.enqueue(it)
	return nil
}

// remove drops a download from the list. Unfinished downloads are stopped
// and their partial files deleted; finished files are kept.
func (a *app) remove(id int) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	a.mu.Lock()
	wasRunning := it.running
	if wasRunning {
		it.requeue = false
		it.cancel()
	}
	a.mu.Unlock()
	for i := 0; wasRunning && i < 50; i++ { // wait for the engine to let go of the file
		a.mu.Lock()
		r := it.running
		a.mu.Unlock()
		if !r {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if s, _ := it.d.State(); s != "done" {
		if p, _ := it.d.Info(); p != "" {
			os.Remove(p + ".part")
			os.Remove(p + ".part.json")
			os.RemoveAll(p + ".parts")
		}
	}
	a.mu.Lock()
	for i, x := range a.items {
		if x == it {
			a.items = append(a.items[:i], a.items[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	a.kick()
	return nil
}

func (a *app) getSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *app) setSettings(s Settings) error {
	if s.OnFinish == "" {
		s.OnFinish = "none"
	}
	if err := s.validate(); err != nil {
		return err
	}
	if s.MiniPos == nil { // the settings page doesn't know about it
		s.MiniPos = a.getSettings().MiniPos
	}
	dir := s.DownloadDir
	if dir == "" {
		dir = a.defaultDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("can't use that download folder: %w", err)
	}
	if a.getSettings().Autostart != s.Autostart || s.Autostart {
		if err := setAutostart(s.Autostart); err != nil { // also refreshes the exe path
			return fmt.Errorf("can't change start with Windows: %w", err)
		}
	}
	if err := s.save(); err != nil {
		return err
	}
	a.eng.SetSortByType(s.SortByType)
	a.eng.SetProxy(s.Proxy)
	if s.Connections > 0 {
		a.eng.SetSegments(s.Connections)
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	a.eng.Limit.SetRate(int64(s.SpeedLimitKB) * 1024)
	a.eng.SetDir(dir)
	log.Printf("settings: %+v", s)
	if a.changed != nil {
		a.changed()
	}
	a.kick()
	return nil
}

// updateSettings changes settings in place.
func (a *app) updateSettings(f func(*Settings)) error {
	s := a.getSettings()
	f(&s)
	return a.setSettings(s)
}

func (a *app) setSpeedLimit(kb int) error {
	s := a.getSettings()
	s.SpeedLimitKB = kb
	return a.setSettings(s)
}

func (a *app) openFile(id int, folder bool) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	p, _ := it.d.Info()
	if s, _ := it.d.State(); s != "done" {
		p += ".part"
	}
	if folder {
		return exec.Command("explorer", "/select,", p).Start()
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
}

func (a *app) openDir() error {
	dir := a.eng.Dir()
	os.MkdirAll(dir, 0o755)
	return exec.Command("explorer", dir).Start()
}

// measure updates each download's speed once a second, and keeps the last
// minute of total speed for the sidebar graph. It also keeps Windows from
// sleeping while anything downloads.
func (a *app) measure() {
	for range time.Tick(time.Second) {
		a.mu.Lock()
		total := 0.0
		for _, it := range a.items {
			n := it.d.Done.Load()
			it.speed = 0.6*float64(n-it.lastDone) + 0.4*it.speed // smoothed
			it.lastDone = n
			if it.running {
				total += it.speed
			}
		}
		a.speedHist = append(a.speedHist, total)
		if len(a.speedHist) > 60 {
			a.speedHist = a.speedHist[len(a.speedHist)-60:]
		}
		busy := a.running > 0
		a.mu.Unlock()
		keepAwake(busy)
		if a.onTick != nil {
			a.onTick()
		}
	}
}

func pct(d *engine.Download) float64 { return d.Progress() }

type row struct {
	ID          int     `json:"id"`
	Status      string  `json:"status"`
	Percent     float64 `json:"percent"`
	Done        int64   `json:"done"`
	Total       int64   `json:"total"`
	Speed       float64 `json:"speed"`
	Connections int     `json:"connections"`
	File        string  `json:"file"`
	URL         string  `json:"url"`
	Kind        string  `json:"kind,omitempty"`
	Category    string  `json:"category"` // Video, Music, Images, ... or ""
	Folder      string  `json:"folder"`
	Added       int64   `json:"added"`    // unix ms
	Finished    int64   `json:"finished"` // unix ms, 0 until done
	Error       string  `json:"error,omitempty"`
	Queue       string  `json:"queue,omitempty"`    // named queue ID
	Verified    bool    `json:"verified,omitempty"` // matched the checksum given when added
	Retrying    bool    `json:"retrying,omitempty"` // waiting after a network drop
	RetryIn     int     `json:"retryIn,omitempty"`  // seconds until the next try
}

func (a *app) rows() []row {
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := []row{}
	for _, it := range a.items {
		d := it.d
		status, err := d.State()
		p, total := d.Info()
		name := filepath.Base(p)
		if p == "" {
			name = d.Req.Filename
			if name == "" {
				name = d.Req.Title
			}
			if name == "" && d.Req.Kind == "" { // not started yet: the name in the link
				if u, err := url.Parse(d.Req.URL); err == nil {
					if b, err := url.PathUnescape(path.Base(u.Path)); err == nil && b != "/" && b != "." {
						name = b
					}
				}
			}
		}
		r := row{ID: d.ID, Status: status, Percent: pct(d), Done: d.Done.Load(), Total: total,
			Connections: d.Connections(), File: name, URL: d.Req.URL, Kind: d.Req.Kind,
			Category: engine.CategoryOf(name), Added: it.added.UnixMilli()}
		if d.Req.Kind != "" {
			r.Category = "Video"
		}
		if p != "" {
			r.Folder = filepath.Dir(p)
		}
		if !it.finished.IsZero() {
			r.Finished = it.finished.UnixMilli()
		}
		if status == "downloading" {
			r.Speed = it.speed
		}
		if err != nil {
			r.Error = err.Error()
		}
		r.Verified = d.Verified
		r.Queue = d.QueueID()
		if status == "queued" && !it.retryAt.IsZero() {
			r.RetryIn = max(int(time.Until(it.retryAt).Seconds()+0.99), 0)
			r.Retrying = true
		}
		rows = append(rows, r)
	}
	return rows
}

// status is everything the window shows, fetched once per refresh.
type status struct {
	Rows    []row     `json:"rows"`
	Summary string    `json:"summary"` // limit / schedule / finish action
	Waiting bool      `json:"waiting"` // queued downloads are held by the schedule
	Speed   []float64 `json:"speed"`   // total speed, last 60 seconds
}

func (a *app) status() status {
	s := a.getSettings()
	now := time.Now()
	a.mu.Lock()
	hist := append([]float64(nil), a.speedHist...)
	a.mu.Unlock()
	return status{Rows: a.rows(), Summary: s.describe(now), Waiting: !s.Schedule.allowed(now), Speed: hist}
}

// handle serves the token-protected API used by velo-host and veloctl.
func (a *app) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || subtle.ConstantTimeCompare([]byte(r.Header.Get(ipc.TokenHeader)), []byte(a.token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	enc := json.NewEncoder(w)
	reply := func(id int, err error) {
		if err != nil {
			enc.Encode(ipc.Reply{Error: err.Error(), Ext: extensionVersion()})
		} else {
			enc.Encode(ipc.Reply{OK: true, ID: id, Ext: extensionVersion()})
		}
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if r.Header.Get("Content-Type") == "application/json" { // from velo-host, i.e. the extension
		a.mu.Lock()
		a.extSeen = time.Now()
		a.mu.Unlock()
	}
	switch r.URL.Path {
	case "/ping":
		reply(0, nil)
	case "/batch":
		var b batchReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&b); err != nil || len(b.Links) == 0 {
			reply(0, fmt.Errorf("no links"))
			return
		}
		if len(b.Links) > 2000 {
			b.Links = b.Links[:2000]
		}
		a.mu.Lock()
		a.batch = &b
		a.mu.Unlock()
		if a.showBatch != nil {
			a.showBatch()
		}
		reply(len(b.Links), nil)
	case "/add":
		var j ipc.Job
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&j); err != nil || j.URL == "" {
			reply(0, fmt.Errorf("bad job"))
			return
		}
		if j.Kind != "" && j.Kind != "hls" && j.Kind != "dash" && j.Kind != "page" {
			reply(0, fmt.Errorf("unknown kind %q", j.Kind))
			return
		}
		r := engine.Request{URL: j.URL, Referrer: j.Referrer, Cookies: j.Cookies,
			UserAgent: j.UserAgent, Filename: j.Filename, Kind: j.Kind, Title: j.Title}
		// Like IDM: a download the browser started opens Velo's download
		// dialog (unless turned off); the browser's own download is stopped.
		if j.Source == "capture" && a.getSettings().AskOnCapture && a.showPending != nil {
			a.mu.Lock()
			if len(a.pending) >= 50 { // a page firing downloads in a loop
				a.mu.Unlock()
				reply(0, fmt.Errorf("too many downloads waiting for the download dialog"))
				return
			}
			a.nextPending++
			a.pending = append(a.pending, pendingDL{ID: a.nextPending, Req: r, Size: j.Size})
			a.mu.Unlock()
			go a.showPending()
			reply(0, nil)
			return
		}
		reply(a.add(r), nil)
	case "/list":
		enc.Encode(a.rows())
	case "/pause":
		reply(id, a.pause(id))
	case "/resume":
		reply(id, a.resume(id))
	case "/remove":
		reply(id, a.remove(id))
	case "/limit": // id = KB/s, 0 = unlimited
		reply(id, a.setSpeedLimit(id))
	case "/quit": // used by the installer before replacing files
		reply(0, nil)
		if a.quit != nil {
			go a.quit()
		}
	case "/version":
		enc.Encode(map[string]string{"version": version.Version})
	case "/show":
		if a.show != nil {
			a.show()
		}
		reply(0, nil)
	default:
		http.NotFound(w, r)
	}
}

var extVersionOnce = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "extension", "manifest.json"))
	if err != nil {
		return "" // source builds run from bin\ without a bundled extension
	}
	var m struct {
		Version string `json:"version"`
	}
	json.Unmarshal(b, &m)
	return m.Version
})

// extensionVersion is the version of the extension installed with Velo, so
// an unpacked copy can tell it's out of date and reload itself.
func extensionVersion() string { return extVersionOnce() }

// lang is the interface language in use.
func (a *app) lang() string { return i18n.Resolve(a.getSettings().Language) }

// tr translates an English interface string.
func (a *app) tr(s string) string { return i18n.T(a.lang(), s) }
