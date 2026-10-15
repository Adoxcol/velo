// Package engine is a segmented HTTP downloader with pause/resume and
// dynamic segmentation.
//
//   - Probe with "Range: bytes=0-0" to learn the size and Range support.
//   - Split the file into N segments; one connection per segment, each
//     writing directly at its offset in a preallocated .part file.
//   - Dynamic segmentation: when a connection finishes its
//     segment, it takes over the back half of the segment with the most
//     bytes left, so fast connections keep working until the very end.
//   - Progress (each segment's pos/end) is saved to <file>.part.json every
//     2s and on pause, so a download resumes after a pause or restart.
//
// Servers without Range support fall back to one stream that can't resume.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MinSplit is the smallest remaining range worth splitting for a new connection.
const MinSplit = 128 << 10

type Request struct {
	URL, Referrer, Cookies, UserAgent, Filename string
	Kind                                        string // "" = file, "hls" = .m3u8 stream, "dash" = .mpd stream
	Title                                       string // page/video title, used to name videos
	Dir                                         string `json:",omitempty"` // save here instead of the default/category folder
	Segments                                    int    `json:",omitempty"` // connections for this download (0 = engine default)
}

// Segment is the byte range [Pos, End] still to fetch. Guarded by Download.mu.
type Segment struct {
	Pos   int64 `json:"pos"`
	End   int64 `json:"end"`
	owned bool  // a connection is working on it
}

// errBusy means the server refused another connection (429/503).
type errBusy struct{ retryAfter time.Duration }

func (e errBusy) Error() string { return "server busy" }

type Download struct {
	ID     int
	Req    Request
	Path   string
	Total  int64 // -1 if unknown
	Ranged bool
	Done   atomic.Int64
	Status string // queued, downloading, paused, done, error
	Err    error

	mu     sync.Mutex
	segs   []*Segment
	active atomic.Int32           // live connections
	direct atomic.Pointer[string] // where the URL redirected to, so segments skip the hop

	parts, partsDone atomic.Int64 // HLS: segments total / on disk
	partBytes        atomic.Int64 // HLS: bytes in the segments on disk
}

func (d *Download) isStream() bool { return d.Req.Kind == "hls" || d.Req.Kind == "dash" }

// Progress returns percent complete (0 if unknown, 100 once done).
func (d *Download) Progress() float64 {
	if s, _ := d.State(); s == "done" {
		return 100
	}
	if n := d.parts.Load(); n > 0 {
		done := d.partsDone.Load()
		if done == n {
			return 100
		}
		if done == 0 || d.partBytes.Load() == 0 {
			return 0
		}
		// Estimate the stream size from the average finished segment.
		est := float64(d.partBytes.Load()) / float64(done) * float64(n)
		return min(99.9, 100*float64(d.Done.Load())/est)
	}
	_, total := d.Info()
	if total <= 0 {
		return 0
	}
	return 100 * float64(d.Done.Load()) / float64(total)
}

// State returns the status and last error; safe to call while Run is active.
func (d *Download) State() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Status, d.Err
}

// Info returns the file path and total size (-1 if unknown); safe while running.
func (d *Download) Info() (path string, total int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Path, d.Total
}

// SetStatus sets the status of a download that isn't running (e.g. queued).
func (d *Download) SetStatus(s string) {
	d.mu.Lock()
	d.Status = s
	d.mu.Unlock()
}

// Connections reports how many connections are currently downloading.
func (d *Download) Connections() int { return int(d.active.Load()) }

type state struct {
	Req      Request    `json:"req"`
	Total    int64      `json:"total"`
	Segments []*Segment `json:"segments"`
}

func statePath(p string) string { return p + ".part.json" }

func (d *Download) saveState() error {
	d.mu.Lock()
	st := state{Req: d.Req, Total: d.Total}
	for _, s := range d.segs {
		if s.Pos <= s.End {
			c := *s
			st.Segments = append(st.Segments, &c)
		}
	}
	d.mu.Unlock()
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := statePath(d.Path) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(d.Path))
}

// Load restores a paused download from its .part.json file.
func Load(stateFile string) (*Download, error) {
	b, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	d := &Download{Req: st.Req, Total: st.Total, Ranged: true, Status: "paused",
		Path: strings.TrimSuffix(stateFile, ".part.json"), segs: st.Segments}
	if st.Req.Kind == "hls" || st.Req.Kind == "dash" { // progress is recounted from <file>.parts when resumed
		d.Ranged, d.segs = false, nil
		return d, nil
	}
	left := int64(0)
	for _, s := range st.Segments {
		left += s.End - s.Pos + 1
	}
	d.Done.Store(st.Total - left)
	return d, nil
}

// FindPaused lists the paused downloads in dir.
func FindPaused(dir string) []*Download {
	files, _ := filepath.Glob(filepath.Join(dir, "*.part.json"))
	var out []*Download
	for _, f := range files {
		if d, err := Load(f); err == nil {
			out = append(out, d)
		}
	}
	return out
}

type Engine struct {
	dir        atomic.Pointer[string] // download folder for new downloads
	sortByType atomic.Bool            // put new downloads in category subfolders
	segments   atomic.Int32           // default connections per download
	Client     *http.Client
	Limit      *Limiter // shared speed limit; nil = unlimited
}

func New(dir string) *Engine {
	e := &Engine{Client: newClient(60 * time.Second), Limit: &Limiter{}}
	e.SetDir(dir)
	e.SetSegments(16)
	return e
}

// Dir is the folder new downloads are saved to. Downloads already started
// keep their own path when it changes.
func (e *Engine) Dir() string     { return *e.dir.Load() }
func (e *Engine) SetDir(d string) { e.dir.Store(&d) }

func (e *Engine) newReq(ctx context.Context, r Request) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.URL, nil)
	if err != nil {
		return nil, err
	}
	if r.UserAgent != "" {
		req.Header.Set("User-Agent", r.UserAgent)
	}
	if r.Referrer != "" {
		req.Header.Set("Referer", r.Referrer)
	}
	if r.Cookies != "" {
		req.Header.Set("Cookie", r.Cookies)
	}
	return req, nil
}

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func fileName(r Request, resp *http.Response) string {
	name := r.Filename // chosen by the user or the browser
	if name == "" {
		if _, p, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil && p["filename"] != "" {
			name = p["filename"]
		}
	}
	if name == "" {
		if u, err := url.Parse(resp.Request.URL.String()); err == nil {
			name, _ = url.PathUnescape(path.Base(u.Path))
		}
	}
	name = strings.TrimSpace(unsafeChars.ReplaceAllString(name, "_"))
	if name == "" || name == "." || name == "_" {
		name = "download"
	}
	return name
}

// uniquePath avoids overwriting existing files: "a.zip" -> "a (1).zip".
func uniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(p + ".part"); errors.Is(err, os.ErrNotExist) {
				return p
			}
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
}

// Run downloads (or resumes) d. Cancelling ctx pauses it: progress is
// saved and Run returns context.Canceled with Status "paused".
func (e *Engine) Run(ctx context.Context, d *Download) error {
	d.mu.Lock()
	d.Status, d.Err = "downloading", nil
	d.mu.Unlock()
	var err error
	switch d.Req.Kind {
	case "hls":
		err = e.runHLS(ctx, d)
	case "dash":
		err = e.runDASH(ctx, d)
	default:
		err = e.run(ctx, d)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil && !d.isStream() && (d.segs == nil || !d.Ranged) {
		// Nothing resumable yet (stopped while probing) or the server can't
		// resume: drop the partial file and start over next time.
		if d.Path != "" {
			os.Remove(d.Path + ".part")
		}
		d.Path, d.Total, d.Ranged, d.segs = "", -1, false, nil
		d.Done.Store(0)
	}
	switch {
	case err == nil:
		d.Status = "done"
	case ctx.Err() != nil:
		d.Status, err = "paused", context.Canceled
	default:
		d.Status, d.Err = "error", err
	}
	return err
}

func (e *Engine) run(ctx context.Context, d *Download) error {
	if d.segs != nil { // resuming
		f, err := os.OpenFile(d.Path+".part", os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		return e.finish(d, f, e.segmented(ctx, d, f))
	}

	req, err := e.newReq(ctx, d.Req)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := e.Client.Do(req)
	if err != nil {
		return err
	}
	total, ranged := int64(-1), false
	if resp.StatusCode == http.StatusPartialContent {
		if cr := resp.Header.Get("Content-Range"); strings.Contains(cr, "/") {
			if n, err := strconv.ParseInt(cr[strings.LastIndex(cr, "/")+1:], 10, 64); err == nil {
				total, ranged = n, true
			}
		}
	} else if resp.StatusCode == http.StatusOK {
		total = resp.ContentLength
	} else {
		resp.Body.Close()
		return fmt.Errorf("server returned %s", resp.Status)
	}

	if u := resp.Request.URL.String(); u != d.Req.URL {
		d.direct.Store(&u)
	}
	name := fileName(d.Req, resp)
	dir := e.targetDir(d.Req, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		resp.Body.Close()
		return err
	}
	d.mu.Lock()
	d.Total, d.Ranged = total, ranged
	d.Path = uniquePath(dir, name)
	d.mu.Unlock()
	f, err := os.Create(d.Path + ".part")
	if err != nil {
		resp.Body.Close()
		return err
	}

	if !d.Ranged {
		d.active.Store(1)
		_, err = io.Copy(f, &counter{r: resp.Body, n: &d.Done, ctx: ctx, lim: e.Limit})
		d.active.Store(0)
		resp.Body.Close()
		return e.finish(d, f, err)
	}
	resp.Body.Close()
	if err := f.Truncate(d.Total); err != nil {
		return e.finish(d, f, err)
	}
	n := int64(e.segmentsFor(d))
	if d.Total < MinSplit*2 {
		n = 1
	}
	size := d.Total / n
	for i := int64(0); i < n; i++ {
		end := (i+1)*size - 1
		if i == n-1 {
			end = d.Total - 1
		}
		d.segs = append(d.segs, &Segment{Pos: i * size, End: end})
	}
	return e.finish(d, f, e.segmented(ctx, d, f))
}

// finish closes f; on success renames .part to the final name and drops the
// state file, on pause/error saves the state for a later resume.
func (e *Engine) finish(d *Download, f *os.File, err error) error {
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if d.Ranged {
			d.saveState()
		}
		return err
	}
	os.Remove(statePath(d.Path))
	return os.Rename(d.Path+".part", d.Path)
}

// steal splits the segment with the most bytes left and returns its back
// half, or nil when nothing is worth splitting.
func (d *Download) steal() *Segment {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.segs { // first, a segment a dropped connection left behind
		if !s.owned && s.Pos <= s.End {
			s.owned = true
			return s
		}
	}
	var big *Segment
	for _, s := range d.segs {
		if !s.owned {
			continue
		}
		if big == nil || s.End-s.Pos > big.End-big.Pos {
			big = s
		}
	}
	if big == nil || big.End-big.Pos+1 < 2*MinSplit {
		return nil
	}
	mid := big.Pos + (big.End-big.Pos+1)/2
	ns := &Segment{Pos: mid, End: big.End, owned: true}
	big.End = mid - 1
	d.segs = append(d.segs, ns)
	return ns
}

func (e *Engine) segmentsFor(d *Download) int {
	if d.Req.Segments > 0 {
		return min(d.Req.Segments, 32)
	}
	return int(e.segments.Load())
}

// SegmentsDefault is the default number of connections per download.
func (e *Engine) SegmentsDefault() int { return int(e.segments.Load()) }

// SetSegments sets the default number of connections per download (1-32).
func (e *Engine) SetSegments(n int) { e.segments.Store(int32(min(max(n, 1), 32))) }

func (e *Engine) segmented(ctx context.Context, d *Download, f *os.File) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	saveDone := make(chan struct{})
	go func() { // periodic progress save
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				d.saveState()
			case <-saveDone:
				return
			}
		}
	}()
	defer close(saveDone)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	var sawBusy atomic.Bool
	var liveMu sync.Mutex
	live, allDone := 0, make(chan struct{})
	var worker func(s *Segment)
	spawn := func(s *Segment) bool { // false once every worker has finished
		liveMu.Lock()
		defer liveMu.Unlock()
		if live < 0 {
			return false
		}
		live++
		wg.Add(1)
		go worker(s)
		return true
	}
	worker = func(s *Segment) {
		defer func() {
			liveMu.Lock()
			if live--; live == 0 {
				live = -1
				close(allDone)
			}
			liveMu.Unlock()
			wg.Done()
		}()
		d.active.Add(1)
		defer d.active.Add(-1)
		busy := 0
		for s != nil {
			err := e.fetch(ctx, d, f, s)
			var b errBusy
			if errors.As(err, &b) {
				sawBusy.Store(true)
				if d.active.Load() > 1 { // others are working: drop this connection
					d.mu.Lock()
					s.owned = false
					d.mu.Unlock()
					return
				}
				if busy++; busy <= 10 { // last connection: wait and retry
					wait := b.retryAfter
					if wait <= 0 {
						wait = time.Duration(busy) * 2 * time.Second
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(wait):
					}
					continue
				}
			}
			if err != nil {
				errOnce.Do(func() { firstErr = err; cancel() })
				return
			}
			busy = 0
			s = d.steal() // dynamic segmentation
		}
	}

	d.mu.Lock()
	var start []*Segment
	for _, s := range d.segs {
		if s.Pos <= s.End {
			s.owned = true
			start = append(start, s)
		}
	}
	d.mu.Unlock()
	for len(start) < e.segmentsFor(d) { // e.g. resumed with fewer segments left
		s := d.steal()
		if s == nil {
			break
		}
		start = append(start, s)
	}
	for _, s := range start {
		spawn(s)
	}
	if len(start) > 0 && d.Req.Segments == 0 {
		wg.Add(1)
		go func() { defer wg.Done(); e.grow(ctx, d, allDone, &sawBusy, spawn) }()
	}
	wg.Wait()
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	if firstErr == nil {
		d.mu.Lock()
		for _, s := range d.segs {
			if s.Pos <= s.End {
				firstErr = fmt.Errorf("incomplete: bytes %d-%d missing", s.Pos, s.End)
				break
			}
		}
		d.mu.Unlock()
	}
	return firstErr
}

// Growth limits: servers that cap each connection (GitHub, many CDNs) get
// extra connections, one at a time, while each one stays slow and adding
// them keeps helping.
var ( // vars so tests can speed them up
	maxConnections int64 = 32
	slowConn       int64 = 1 << 20 // bytes/s per connection below which we add more
	growEvery            = 3 * time.Second
)

// grow adds connections to a running download while that raises its speed.
// It stops for good when the server reports it is busy or a new connection
// did not help, and pauses while a speed limit is set.
func (e *Engine) grow(ctx context.Context, d *Download, allDone <-chan struct{}, busy *atomic.Bool, spawn func(*Segment) bool) {
	t := time.NewTicker(growEvery)
	defer t.Stop()
	last, lastSpeed := d.Done.Load(), int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-allDone:
			return
		case <-t.C:
		}
		done := d.Done.Load()
		speed := (done - last) * int64(time.Second) / int64(growEvery)
		last = done
		if busy.Load() {
			return
		}
		n := int64(d.active.Load())
		if (e.Limit != nil && e.Limit.Rate() > 0) || n == 0 || n >= maxConnections {
			continue
		}
		if lastSpeed > 0 && speed < lastSpeed*21/20 { // the last one added less than 5%
			return
		}
		if speed/n >= slowConn {
			lastSpeed = 0
			continue
		}
		s := d.steal()
		if s == nil {
			continue
		}
		if !spawn(s) {
			d.mu.Lock()
			s.owned = false
			d.mu.Unlock()
			return
		}
		lastSpeed = speed
	}
}

// segReq builds a segment request, going straight to the redirect target
// when the probe was redirected (GitHub releases, CDNs). Cookies are only
// sent there if it is the same host, like a redirect would.
func (e *Engine) segReq(ctx context.Context, d *Download) (*http.Request, bool, error) {
	u := d.direct.Load()
	if u == nil {
		req, err := e.newReq(ctx, d.Req)
		return req, false, err
	}
	r := d.Req
	r.URL = *u
	if a, b := hostOf(d.Req.URL), hostOf(*u); a != b && !strings.HasSuffix(b, "."+a) {
		r.Cookies = ""
	}
	req, err := e.newReq(ctx, r)
	return req, true, err
}

func hostOf(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// fetch downloads s until s.Pos passes s.End (End can shrink while it runs
// when another connection steals part of it). Retries resume from s.Pos.
func (e *Engine) fetch(ctx context.Context, d *Download, f *os.File, s *Segment) error {
	buf := make([]byte, 64<<10)
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		d.mu.Lock()
		pos, end := s.Pos, s.End
		d.mu.Unlock()
		if pos > end {
			return nil
		}
		req, direct, err := e.segReq(ctx, d)
		if err != nil {
			return err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, end))
		resp, err := e.Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			secs, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			return errBusy{time.Duration(secs) * time.Second}
		}
		if direct && resp.StatusCode >= 400 { // the redirect link expired: go through the original URL again
			resp.Body.Close()
			d.direct.Store(nil)
			attempt--
			continue
		}
		if resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return fmt.Errorf("range %d-%d: %s", pos, end, resp.Status)
		}
		finished, err := d.copySeg(ctx, e.Limit, f, s, resp.Body, buf)
		resp.Body.Close()
		if finished {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = err
		if lastErr == nil {
			lastErr = io.ErrUnexpectedEOF
		}
	}
	return fmt.Errorf("segment failed: %w", lastErr)
}

// copySeg writes body into f at s.Pos, stopping at s.End. Writes happen
// under d.mu so a saved state never claims bytes that aren't on disk and a
// concurrent steal can't move End below data being written.
func (d *Download) copySeg(ctx context.Context, lim *Limiter, f *os.File, s *Segment, body io.Reader, buf []byte) (bool, error) {
	for {
		n, rerr := body.Read(buf[:lim.chunk(len(buf))])
		if n > 0 {
			if err := lim.Wait(ctx, n); err != nil { // outside d.mu
				return false, err
			}
			d.mu.Lock()
			if s.Pos > s.End {
				d.mu.Unlock()
				return true, nil
			}
			if left := s.End - s.Pos + 1; int64(n) > left {
				n = int(left)
			}
			_, werr := f.WriteAt(buf[:n], s.Pos)
			if werr == nil {
				s.Pos += int64(n)
			}
			done := s.Pos > s.End
			d.mu.Unlock()
			if werr != nil {
				return false, werr
			}
			d.Done.Add(int64(n))
			if done {
				return true, nil
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				rerr = nil
			}
			return false, rerr
		}
	}
}

type counter struct {
	r   io.Reader
	n   *atomic.Int64
	ctx context.Context
	lim *Limiter
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p[:c.lim.chunk(len(p))])
	if n > 0 {
		if werr := c.lim.Wait(c.ctx, n); werr != nil {
			return n, werr
		}
	}
	c.n.Add(int64(n))
	return n, err
}
