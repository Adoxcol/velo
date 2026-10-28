package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testServer serves data with Range support. Each response is throttled,
// and more than maxConns concurrent requests get 429 (0 = unlimited).
func testServer(t *testing.T, data []byte, maxConns int32, bytesPerSec int) (*httptest.Server, *atomic.Int32) {
	var live, rejected atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := live.Add(1); maxConns > 0 && n > maxConns {
			live.Add(-1)
			rejected.Add(1)
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		defer live.Add(-1)
		http.ServeContent(&slowWriter{w, bytesPerSec}, r, "file.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	return srv, &rejected
}

type slowWriter struct {
	http.ResponseWriter
	bps int
}

func (s *slowWriter) Write(p []byte) (int, error) {
	if s.bps <= 0 {
		return s.ResponseWriter.Write(p)
	}
	const chunk = 16 << 10
	total := 0
	for len(p) > 0 {
		n := min(chunk, len(p))
		w, err := s.ResponseWriter.Write(p[:n])
		total += w
		if err != nil {
			return total, err
		}
		p = p[n:]
		time.Sleep(time.Duration(n) * time.Second / time.Duration(s.bps))
	}
	return total, nil
}

func randomData(t *testing.T, n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func check(t *testing.T, d *Download, want []byte) {
	t.Helper()
	got, err := os.ReadFile(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content mismatch (%d vs %d bytes)", len(got), len(want))
	}
	if _, err := os.Stat(statePath(d.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("state file left behind")
	}
}

func TestSegmented(t *testing.T) {
	data := randomData(t, 8<<20)
	srv, _ := testServer(t, data, 0, 0)
	e := New(t.TempDir())
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
}

func TestPauseResumeAcrossRestart(t *testing.T) {
	data := randomData(t, 8<<20)
	srv, _ := testServer(t, data, 0, 512<<10) // ~0.5 MB/s per connection
	dir := t.TempDir()
	e := New(dir)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	if err := e.Run(ctx, d); !errors.Is(err, context.Canceled) || d.Status != "paused" {
		t.Fatalf("want paused, got %v / %s", err, d.Status)
	}
	paused := FindPaused(dir) // as if the app restarted
	if len(paused) != 1 {
		t.Fatalf("want 1 paused download, got %d", len(paused))
	}
	r := paused[0]
	if got := r.Done.Load(); got <= 0 || got >= int64(len(data)) {
		t.Fatalf("resume point %d out of range", got)
	}
	t.Logf("paused at %d/%d bytes", r.Done.Load(), len(data))
	if err := New(dir).Run(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	check(t, r, data)
}

func TestDynamicSegmentation(t *testing.T) {
	d := &Download{segs: []*Segment{
		{Pos: 0, End: 10<<20 - 1, owned: true},
		{Pos: 10 << 20, End: 10<<20 + 100, owned: true},
	}}
	s := d.steal()
	if s == nil || s.Pos != 5<<20 || s.End != 10<<20-1 || d.segs[0].End != 5<<20-1 {
		t.Fatalf("bad split: %+v / %+v", s, d.segs[0])
	}
	// Too small to split further after a few rounds.
	for d.steal() != nil {
	}
	for _, s := range d.segs {
		if s.End-s.Pos+1 >= 2*MinSplit {
			t.Fatalf("segment %+v should have been split", s)
		}
	}
}

// Server allows 3 connections; the engine opens 8. Rejected connections
// must drop out and the remaining ones must finish everything.
func TestBusyServer(t *testing.T) {
	data := randomData(t, 6<<20)
	srv, rejected := testServer(t, data, 3, 8<<20)
	e := New(t.TempDir())
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
	if rejected.Load() == 0 {
		t.Fatal("expected the server to reject some connections")
	}
	t.Logf("%d connections rejected", rejected.Load())
}

func TestNoRangeSupport(t *testing.T) {
	data := randomData(t, 3<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, bytes.NewReader(data)) // ignores Range, sends 200
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := &Download{Req: Request{URL: srv.URL + "/plain.bin"}}
	if err := New(dir).Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(d.Path) != "plain.bin" {
		t.Fatalf("name %s", d.Path)
	}
	check(t, d, data)
}

// A 1 MB download limited to 512 KB/s should take about 2s (minus the burst).
func TestSpeedLimit(t *testing.T) {
	data := randomData(t, 1<<20)
	srv, _ := testServer(t, data, 0, 0)
	e := New(t.TempDir())
	e.Limit.SetRate(512 << 10)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	start := time.Now()
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	el := time.Since(start)
	check(t, d, data)
	if el < 1500*time.Millisecond || el > 3*time.Second {
		t.Fatalf("took %v, want ~2s", el)
	}
	t.Logf("1 MB at 512 KB/s took %v", el.Round(10*time.Millisecond))
}

// Cancelling during the probe leaves no files and restarts cleanly.
func TestCancelBeforeSegments(t *testing.T) {
	data := randomData(t, 2<<20)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
		http.ServeContent(w, r, "x.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()
	dir := t.TempDir()
	e := New(dir)
	d := &Download{Req: Request{URL: srv.URL + "/x.bin"}}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	if err := e.Run(ctx, d); !errors.Is(err, context.Canceled) || d.Status != "paused" {
		t.Fatalf("want paused, got %v / %s", err, d.Status)
	}
	close(block)
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
	if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 1 {
		t.Fatalf("want just the finished file, got %v", files)
	}
}

// A segment connection that stops sending is dropped and retried.
func TestStalledConnectionRetries(t *testing.T) {
	data := randomData(t, 4<<20)
	var stalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-0" && stalls.Add(1) == 1 {
			w.Header().Set("Content-Range", "bytes 0-0/1") // first segment request: send headers, then hang
			w.WriteHeader(http.StatusPartialContent)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		http.ServeContent(w, r, "s.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()
	e := New(t.TempDir())
	e.Client = newClient(300*time.Millisecond, nil)
	d := &Download{Req: Request{URL: srv.URL + "/s.bin"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
}

// A server that caps each connection gets extra connections, and segments
// go straight to the redirect target instead of through the redirect.
func TestGrowAndRedirect(t *testing.T) {
	oldEvery, oldSlow := growEvery, slowConn
	growEvery, slowConn = 200*time.Millisecond, 1<<20
	t.Cleanup(func() { growEvery, slowConn = oldEvery, oldSlow })

	data := randomData(t, 6<<20)
	srv, _ := testServer(t, data, 0, 256<<10) // 256 KB/s per connection
	var hops atomic.Int32
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, srv.URL+"/file.bin", http.StatusFound)
	}))
	t.Cleanup(front.Close)

	e := New(t.TempDir())
	e.SetSegments(2)
	d := &Download{Req: Request{URL: front.URL + "/get", Cookies: "secret=1"}}
	var peak atomic.Int32
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				if n := d.active.Load(); n > peak.Load() {
					peak.Store(n)
				}
			}
		}
	}()
	err := e.Run(context.Background(), d)
	close(stop)
	if err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
	if p := peak.Load(); p <= 2 {
		t.Errorf("connections never grew past 2 (peak %d)", p)
	}
	if h := hops.Load(); h != 1 {
		t.Errorf("redirect followed %d times, want once (the probe)", h)
	}
}

func TestDiskFull(t *testing.T) {
	old := freeSpace
	freeSpace = func(string) int64 { return 1 << 20 }
	t.Cleanup(func() { freeSpace = old })
	data := randomData(t, 2<<20)
	srv, _ := testServer(t, data, 0, 0)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	err := New(t.TempDir()).Run(context.Background(), d)
	var full *DiskFullError
	if !errors.As(err, &full) || Transient(err) {
		t.Fatalf("want a disk full error, got %v", err)
	}
	t.Log(err)
	if free := old(t.TempDir()); free <= 0 {
		t.Fatalf("real free space looks wrong: %d", free)
	}
}

func TestSafeName(t *testing.T) {
	long := strings.Repeat("a", 300) + ".zip"
	for in, want := range map[string]string{
		"..":              "download",
		"...":             "download",
		"../../evil.exe":  ".._.._evil.exe",
		`..\..\evil.exe`:  ".._.._evil.exe",
		"C:evil.txt":      "C_evil.txt",
		"file.txt:stream": "file.txt_stream",
		"CON":             "_CON",
		"nul.txt":         "_nul.txt",
		"COM1.zip":        "_COM1.zip",
		"console.txt":     "console.txt",
		"report.pdf. . .": "report.pdf",
		"  spaced.zip  ":  "spaced.zip",
		"a\x00b\x1fc.bin": "a_b_c.bin",
		"":                "download",
		long:              strings.Repeat("a", 176) + ".zip",
	} {
		if got := safeName(in, "download"); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateFileHasNoCookies(t *testing.T) {
	dir := t.TempDir()
	d := &Download{Req: Request{URL: "http://x/f", Cookies: "session=secret"}, Total: 10, Path: filepath.Join(dir, "f"),
		segs: []*Segment{{Pos: 0, End: 9}}}
	if err := d.saveState(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(statePath(d.Path))
	if bytes.Contains(b, []byte("secret")) {
		t.Fatalf("cookies written to %s: %s", statePath(d.Path), b)
	}
	if d.Req.Cookies == "" {
		t.Fatal("saveState cleared the live download's cookies")
	}
}

func TestChecksum(t *testing.T) {
	data := randomData(t, 1<<20)
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])
	srv, _ := testServer(t, data, 0, 0)
	e := New(t.TempDir())

	parsed, err := ParseChecksum("SHA256:" + strings.ToUpper(good) + "  file.bin")
	if err != nil || parsed != good {
		t.Fatalf("ParseChecksum: %q %v", parsed, err)
	}
	if _, err := ParseChecksum("not-a-hash"); err == nil {
		t.Fatal("accepted a bad checksum")
	}

	d := &Download{Req: Request{URL: srv.URL + "/file.bin", Checksum: good}}
	if err := e.Run(context.Background(), d); err != nil || !d.Verified {
		t.Fatalf("good checksum: err=%v verified=%v", err, d.Verified)
	}

	bad := &Download{Req: Request{URL: srv.URL + "/file.bin", Checksum: strings.Repeat("0", 64)}}
	err = e.Run(context.Background(), bad)
	var ce *ChecksumError
	if !errors.As(err, &ce) || bad.Verified || Transient(err) {
		t.Fatalf("bad checksum: %v", err)
	}
	if p, _ := bad.Info(); p == "" {
		t.Fatal("file should be kept for inspection")
	} else if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

// Downloads go through a custom proxy, and "none" goes direct.
func TestProxy(t *testing.T) {
	t.Run("download via proxy", func(t *testing.T) {
		data := randomData(t, 1<<20)
		var proxied atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !r.URL.IsAbs() {
				http.Error(w, "not a proxy request", http.StatusBadRequest)
				return
			}
			proxied.Add(1)
			http.ServeContent(w, r, "f.bin", time.Time{}, bytes.NewReader(data))
		}))
		t.Cleanup(proxy.Close)

		e := New(t.TempDir())
		p, err := ParseProxy(strings.TrimPrefix(proxy.URL, "http://")) // host:port is enough
		if err != nil {
			t.Fatal(err)
		}
		e.SetProxy(p)
		d := &Download{Req: Request{URL: "http://example.invalid/f.bin"}} // only the proxy can serve this
		if err := e.Run(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		check(t, d, data)
		if proxied.Load() == 0 {
			t.Fatal("nothing went through the proxy")
		}
	})

	t.Run("invalid proxy inputs", func(t *testing.T) {
		for _, bad := range []string{"ftp://x:1", "http://nohost", "::"} {
			if _, err := ParseProxy(bad); err == nil {
				t.Errorf("ParseProxy(%q) accepted", bad)
			}
		}
	})

	t.Run("wildcard match", func(t *testing.T) {
		if !wildcardMatch("*.corp.com", "a.corp.com") || wildcardMatch("*.corp.com", "corp.org") || !wildcardMatch("10.*", "10.1.2.3") {
			t.Error("wildcardMatch failed")
		}
	})

	t.Run("parse proxy server", func(t *testing.T) {
		if m := parseProxyServer("http=h:1;https=h:2;socks=s:3"); m["https"] != "h:2" || m["socks"] != "s:3" {
			t.Errorf("parseProxyServer: %v", m)
		}
	})
}

// A login is sent to the site that needs it, never to another host the
// download is redirected to.
func TestBasicAuth(t *testing.T) {
	data := randomData(t, 2<<20)
	var leaked atomic.Bool
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
		http.ServeContent(w, r, "f.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(cdn.Close)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ann" || p != "pw" {
			w.Header().Set("WWW-Authenticate", `Basic realm="files"`)
			http.Error(w, "login", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, strings.Replace(cdn.URL, "127.0.0.1", "localhost", 1)+"/f.bin", http.StatusFound)
	}))
	t.Cleanup(site.Close)

	e := New(t.TempDir())
	if _, err := e.Probe(context.Background(), Request{URL: site.URL + "/f"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("probe without login: %v", err)
	}
	d := &Download{Req: Request{URL: site.URL + "/f", User: "ann", Pass: "pw"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
	if leaked.Load() {
		t.Fatal("the login was sent to the other host")
	}
}

// A file replaced on the server while paused (same size, new ETag) must not
// be stitched into the old one: the resume fails instead.
func TestResumeRejectsChangedFile(t *testing.T) {
	data := randomData(t, 8<<20)
	var etag atomic.Value
	etag.Store(`"v1"`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag.Load().(string))
		http.ServeContent(&slowWriter{w, 512 << 10}, r, "file.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	if err := New(dir).Run(ctx, d); !errors.Is(err, context.Canceled) {
		t.Fatalf("want paused, got %v", err)
	}
	etag.Store(`"v2"`)
	paused := FindPaused(dir)
	if len(paused) != 1 || paused[0].Validator != `"v1"` {
		t.Fatalf("validator not saved: %+v", paused)
	}
	err := New(dir).Run(context.Background(), paused[0])
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusPreconditionFailed {
		t.Fatalf("want changed-file error, got %v", err)
	}
}

// A server that allows only a few connections and refuses the rest with 403
// (not 429) still completes: refused extra connections are dropped.
func TestRefusedExtraConnectionsAreDropped(t *testing.T) {
	data := randomData(t, 8<<20)
	var live atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-0" {
			if n := live.Add(1); n > 2 {
				live.Add(-1)
				http.Error(w, "too many connections", http.StatusForbidden)
				return
			}
			defer live.Add(-1)
		}
		http.ServeContent(&slowWriter{w, 2 << 20}, r, "file.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	if err := New(t.TempDir()).Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	check(t, d, data)
}

// A server that stops answering partway through fails the download quickly,
// with a message that says what happened.
func TestServerGoneFailsQuickly(t *testing.T) {
	data := randomData(t, 8<<20)
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(&slowWriter{w, 512 << 10}, r, "file.bin", time.Time{}, bytes.NewReader(data))
	})
	srv.Start()
	time.AfterFunc(500*time.Millisecond, func() { srv.Listener.Close(); srv.CloseClientConnections() })
	t.Cleanup(srv.Close)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin"}}
	start := time.Now()
	err := New(t.TempDir()).Run(context.Background(), d)
	t.Logf("after %v: %v", time.Since(start).Round(time.Millisecond), err)
	if err == nil {
		t.Fatal("want an error")
	}
	if !Transient(err) {
		t.Errorf("a lost server should be retried later: %v", err)
	}
	if time.Since(start) > 30*time.Second {
		t.Errorf("took %v to give up", time.Since(start))
	}
}
