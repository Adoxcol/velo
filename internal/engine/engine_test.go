package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	e.Client = newClient(300 * time.Millisecond)
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
