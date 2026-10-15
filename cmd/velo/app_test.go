package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"velo/internal/engine"
)

func slowServer(t *testing.T, size int) string {
	data := make([]byte, size)
	rand.Read(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "f.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/f.bin"
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func statusOf(a *app, id int) string {
	s, _ := a.get(id).d.State()
	return s
}

func testApp(t *testing.T, s Settings) *app {
	home := t.TempDir() // keep history.json out of the real profile
	t.Setenv("VELO_HOME", home)
	eng := engine.New(t.TempDir())
	a := &app{eng: eng, wake: make(chan struct{}, 1), settings: s, home: home}
	eng.Limit.SetRate(int64(s.SpeedLimitKB) * 1024)
	go a.dispatcher()
	return a
}

func TestScheduleHoldsAndReleasesQueue(t *testing.T) {
	url := slowServer(t, 4<<20)
	s := defaultSettings()
	now := time.Now()
	closed := Schedule{Enabled: true, Start: now.Add(2 * time.Hour).Format("15:04"),
		Stop: now.Add(3 * time.Hour).Format("15:04"), Days: [7]bool{true, true, true, true, true, true, true}}
	s.Schedule = closed
	s.SpeedLimitKB = 1024 // 1 MB/s so downloads are still running when we check
	a := testApp(t, s)

	id := a.add(engine.Request{URL: url})
	time.Sleep(200 * time.Millisecond)
	if st := statusOf(a, id); st != "queued" {
		t.Fatalf("outside the window: want queued, got %s", st)
	}

	// Open the window: the download starts.
	a.mu.Lock()
	a.settings.Schedule.Enabled = false
	a.mu.Unlock()
	a.kick()
	waitFor(t, "download to make progress", func() bool { return a.get(id).d.Done.Load() > 256<<10 })

	// Close the window mid-download: back to the queue, not paused.
	a.mu.Lock()
	a.settings.Schedule = closed
	a.mu.Unlock()
	a.kick()
	waitFor(t, "download to be requeued", func() bool { return statusOf(a, id) == "queued" })
	if p := pct(a.get(id).d); p <= 0 || p >= 100 {
		t.Fatalf("want partial progress, got %.1f%%", p)
	}

	// Reopen: it resumes and finishes.
	a.mu.Lock()
	a.settings.Schedule.Enabled = false
	a.settings.SpeedLimitKB = 0
	a.mu.Unlock()
	a.eng.Limit.SetRate(0)
	a.kick()
	waitFor(t, "download to finish", func() bool { return statusOf(a, id) == "done" })
}

func TestMaxConcurrent(t *testing.T) {
	url := slowServer(t, 2<<20)
	s := defaultSettings()
	s.MaxConcurrent = 2
	s.SpeedLimitKB = 512
	a := testApp(t, s)
	ids := []int{a.add(engine.Request{URL: url}), a.add(engine.Request{URL: url}), a.add(engine.Request{URL: url})}
	waitFor(t, "two downloads running", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.running == 2
	})
	if st := statusOf(a, ids[2]); st != "queued" {
		t.Fatalf("third download should wait, got %s", st)
	}
	a.mu.Lock()
	a.settings.MaxConcurrent = 3
	a.mu.Unlock()
	a.kick()
	waitFor(t, "third download to start", func() bool { return statusOf(a, ids[2]) == "downloading" })
	for _, id := range ids {
		a.pause(id)
	}
	waitFor(t, "all paused", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.running == 0
	})
}

func TestHistoryRoundTrip(t *testing.T) {
	url := slowServer(t, 1<<20)
	a := testApp(t, defaultSettings())
	done := a.add(engine.Request{URL: url, Filename: "a.bin"})
	waitFor(t, "download to finish", func() bool { return statusOf(a, done) == "done" })
	a.register(&engine.Download{Req: engine.Request{URL: "http://127.0.0.1:1/x"}, Status: "paused"})
	a.saveHistory()

	// "Restart": a new app reading the same history.
	b := &app{eng: a.eng, wake: make(chan struct{}, 1), settings: defaultSettings(), home: a.home}
	b.loadHistory(b.readHistory(), nil)
	rows := b.rows()
	if len(rows) != 2 {
		t.Fatalf("want 2 restored downloads, got %d", len(rows))
	}
	if rows[0].Status != "done" || rows[0].Percent != 100 || rows[0].File != "a.bin" {
		t.Fatalf("finished download restored wrong: %+v", rows[0])
	}
	if rows[1].Status != "paused" {
		t.Fatalf("paused download restored as %s", rows[1].Status)
	}
	if n := b.clearDone(); n != 1 || len(b.rows()) != 1 {
		t.Fatalf("clearDone removed %d, %d left", n, len(b.rows()))
	}
}

// Changing the download folder: new downloads go to the new folder, and a
// paused download in the old folder is still found after a restart.
func TestDownloadFolderChange(t *testing.T) {
	url := slowServer(t, 2<<20)
	oldDir, newDir := t.TempDir(), filepath.Join(t.TempDir(), "new folder")
	a := testApp(t, defaultSettings())
	a.eng.SetDir(oldDir)
	a.defaultDir = oldDir

	// A download paused part-way in the old folder.
	a.eng.Limit.SetRate(256 << 10)
	paused := a.add(engine.Request{URL: url, Filename: "old.bin"})
	waitFor(t, "progress", func() bool { return a.get(paused).d.Done.Load() > 64<<10 })
	a.pause(paused)
	waitFor(t, "pause", func() bool { return statusOf(a, paused) == "paused" })
	a.eng.Limit.SetRate(0)

	s := a.getSettings()
	s.DownloadDir = newDir
	if err := a.setSettings(s); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(newDir); err != nil || !fi.IsDir() {
		t.Fatal("new folder wasn't created")
	}
	fresh := a.add(engine.Request{URL: url, Filename: "new.bin"})
	waitFor(t, "new download", func() bool { return statusOf(a, fresh) == "done" })
	if p, _ := a.get(fresh).d.Info(); filepath.Dir(p) != newDir {
		t.Fatalf("new download saved to %s", p)
	}
	a.saveHistory()

	// Restart with the new folder as the setting.
	eng := engine.New(newDir)
	b := &app{eng: eng, wake: make(chan struct{}, 1), settings: s, defaultDir: newDir, home: a.home}
	b.restore()
	var found *item
	for _, it := range b.items {
		if p, _ := it.d.Info(); filepath.Base(p) == "old.bin" {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("paused download in the old folder was lost; restored %d items", len(b.items))
	}
	if st, _ := found.d.State(); st != "paused" {
		t.Fatalf("restored as %s", st)
	}
	// Resuming finishes it in its original folder.
	if err := eng.Run(context.Background(), found.d); err != nil {
		t.Fatal(err)
	}
	if p, _ := found.d.Info(); filepath.Dir(p) != oldDir {
		t.Fatalf("resumed download moved to %s", p)
	}
}

func TestBatchCookiesStayOnSameSite(t *testing.T) {
	a := testApp(t, defaultSettings())
	var b batchReq
	b.Page, b.Cookies = "https://files.example.com/list", "session=secret"
	b.Links = append(b.Links, struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}{URL: "https://files.example.com/a.zip"}, struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}{URL: "https://other.example.net/b.zip"})
	if n := a.addBatch(b, []string{b.Links[0].URL, b.Links[1].URL}, false); n != 2 {
		t.Fatalf("added %d", n)
	}
	for _, it := range a.items {
		same := it.d.Req.URL == b.Links[0].URL
		if (it.d.Req.Cookies != "") != same {
			t.Fatalf("%s got cookies %q", it.d.Req.URL, it.d.Req.Cookies)
		}
	}
}

// A download that loses the network waits and resumes by itself; a missing
// file fails straight away.
func TestRetriesAfterNetworkDrop(t *testing.T) {
	old := retryDelay
	retryDelay = func(int) time.Duration { return 100 * time.Millisecond }
	t.Cleanup(func() { retryDelay = old })

	data := make([]byte, 2<<20)
	rand.Read(data)
	var down atomic.Bool
	down.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		if down.Load() { // drop the connection like a lost network
			c, _, _ := w.(http.Hijacker).Hijack()
			c.Close()
			return
		}
		http.ServeContent(w, r, "f.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)

	a := testApp(t, defaultSettings())
	id := a.add(engine.Request{URL: srv.URL + "/f.bin"})
	waitFor(t, "a retry to be scheduled", func() bool {
		for _, r := range a.rows() {
			if r.ID == id && r.Retrying {
				return true
			}
		}
		return false
	})
	down.Store(false)
	waitFor(t, "download to finish after the network is back", func() bool { return statusOf(a, id) == "done" })
	got, _ := os.ReadFile(func() string { p, _ := a.get(id).d.Info(); return p }())
	if !bytes.Equal(got, data) {
		t.Fatal("content mismatch")
	}

	miss := a.add(engine.Request{URL: srv.URL + "/missing"})
	waitFor(t, "404 to fail", func() bool { return statusOf(a, miss) == "error" })
}
