package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

// batchReq is a list of links sent by "Download all with Velo" or a dropped .txt file.
type batchReq struct {
	Page      string `json:"page"`
	Referrer  string `json:"referrer"`
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
	FromList  bool   `json:"-"` // dropped/imported link list: pre-select all valid URLs
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

// loadBatchLinks populates a pending batch from a dropped or pasted list of URLs.
func (a *app) loadBatchLinks(urls []string, source string) int {
	seen := map[string]bool{}
	b := &batchReq{Page: source, FromList: true}
	if b.Page == "" {
		b.Page = "link list"
	}
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" || seen[u] {
			continue
		}
		if lu := strings.ToLower(u); !strings.HasPrefix(lu, "http://") && !strings.HasPrefix(lu, "https://") {
			continue
		}
		seen[u] = true
		b.Links = append(b.Links, struct {
			URL  string `json:"url"`
			Text string `json:"text"`
		}{URL: u, Text: linkName(u)})
		if len(b.Links) >= 2000 {
			break
		}
	}
	if len(b.Links) == 0 {
		return 0
	}
	a.mu.Lock()
	a.batch = b
	a.mu.Unlock()
	return len(b.Links)
}

// expandPlaylistToBatch resolves a YouTube playlist URL via yt-dlp and populates
// a pending batch so the user can pick which videos in the playlist to download.
func (a *app) expandPlaylistToBatch(rawURL string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	title, entries, err := engine.ExpandPlaylist(ctx, strings.TrimSpace(rawURL))
	if err != nil {
		return 0, err
	}
	pageLabel := title
	if pageLabel == "" {
		pageLabel = rawURL
	}
	b := &batchReq{Page: pageLabel, Referrer: rawURL, FromList: true}
	for _, e := range entries {
		b.Links = append(b.Links, struct {
			URL  string `json:"url"`
			Text string `json:"text"`
		}{URL: e.URL, Text: e.Title})
		if len(b.Links) >= 2000 {
			break
		}
	}
	a.mu.Lock()
	a.batch = b
	a.mu.Unlock()
	return len(b.Links), nil
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
		if b.FromList {
			ok = true
		}
		name := linkName(l.URL)
		cat := engine.CategoryOf(name)
		if kind != "" {
			cat = "Video"
			if l.Text != "" && l.Text != name {
				name = l.Text
			}
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
	titles := map[string]string{}
	for _, l := range b.Links {
		if l.Text != "" {
			titles[l.URL] = l.Text
		}
	}
	n := 0
	for _, u := range urls {
		ok, kind := engine.Downloadable(u)
		// Security: Input validation to ensure only HTTP/HTTPS URLs are accepted.
		lu := strings.ToLower(u)
		if !ok && !strings.HasPrefix(lu, "http://") && !strings.HasPrefix(lu, "https://") {
			continue
		}
		r := engine.Request{URL: u, Referrer: b.Referrer, UserAgent: b.UserAgent, Kind: kind}
		if kind != "" && titles[u] != "" && titles[u] != linkName(u) {
			r.Title = titles[u]
		}
		if sameHost(u, b.Page) { // never send the page's cookies to other sites
			r.Cookies = b.Cookies
		}
		if start {
			if id := a.add(r); id != 0 {
				n++
			}
		} else {
			a.register(&engine.Download{Req: r, Status: "paused"})
			n++
		}
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
	Format   string `json:"format"`
}

// clearPending drops every browser download waiting for the dialog.
func (a *app) clearPending() {
	a.mu.Lock()
	a.pending = nil
	a.pendingOrder = nil
	a.mu.Unlock()
}

// nextPendingDL returns the oldest pending browser download, or nil.
func (a *app) nextPendingDL() *pendingView {
	a.mu.Lock()
	defer a.mu.Unlock()
	for len(a.pendingOrder) > 0 {
		id := a.pendingOrder[0]
		p, ok := a.pending[id]
		if !ok {
			a.pendingOrder = a.pendingOrder[1:]
			continue
		}
		v := &pendingView{ID: p.ID, URL: p.Req.URL, Filename: p.Req.Filename, Size: p.Size, Kind: p.Req.Kind, Format: p.Req.Format}
		if u, err := url.Parse(p.Req.Referrer); err == nil && u.Host != "" {
			v.Site = u.Host
		} else if u, err := url.Parse(p.Req.URL); err == nil {
			v.Site = u.Host
		}
		return v
	}
	return nil
}

// takePending removes a pending download and returns its request.
func (a *app) takePending(id int) (engine.Request, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.pending[id]
	if !ok {
		return engine.Request{}, false
	}
	delete(a.pending, id)
	for len(a.pendingOrder) > 0 && a.pendingOrder[0] == id {
		a.pendingOrder = a.pendingOrder[1:]
	}
	if len(a.pending) == 0 {
		a.pendingOrder = nil
	}
	return p.Req, true
}

// handle serves the token-protected API used by velo-host and veloctl.
func (a *app) handle(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get(ipc.TokenHeader)
	// Security: Ensure both incoming token and app token are non-empty before
	// ConstantTimeCompare, as subtle.ConstantTimeCompare([], []) returns 1,
	// which would allow unauthenticated access if either token is empty.
	if r.Method != "POST" || token == "" || a.token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(a.token)) != 1 {
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
		// Security: Input validation to ensure only HTTP/HTTPS URLs are accepted.
		u := strings.ToLower(j.URL)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			reply(0, fmt.Errorf("invalid URL scheme"))
			return
		}
		if j.Kind != "" && j.Kind != "hls" && j.Kind != "dash" && j.Kind != "page" {
			reply(0, fmt.Errorf("unknown kind %q", j.Kind))
			return
		}
		r := engine.Request{URL: j.URL, Referrer: j.Referrer, Cookies: j.Cookies,
			UserAgent: j.UserAgent, Filename: j.Filename, Kind: j.Kind, Format: j.Format, Title: j.Title}
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
			if a.pending == nil {
				a.pending = make(map[int]pendingDL)
			}
			p := pendingDL{ID: a.nextPending, Req: r, Size: j.Size}
			a.pending[p.ID] = p
			a.pendingOrder = append(a.pendingOrder, p.ID)
			a.mu.Unlock()
			go func() {
				if r.Kind == "page" && r.Filename == "" {
					if name, err := a.eng.VideoFilename(context.Background(), r); err == nil {
						a.mu.Lock()
						if pending, ok := a.pending[p.ID]; ok {
							pending.Req.Filename = name
							a.pending[p.ID] = pending
						}
						a.mu.Unlock()
					}
				}
				a.showPending()
			}()
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
