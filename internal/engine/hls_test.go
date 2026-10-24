package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// hlsServer serves a master playlist with a low and a high variant, each
// with n segments. Segment bodies are distinct per variant. delay slows
// segment responses.
func hlsServer(t *testing.T, n int, media func(variant string) string, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	var segHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/master.m3u8":
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=500000,RESOLUTION=640x360\nlow/index.m3u8\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080\nhigh/index.m3u8\n")
		case strings.HasSuffix(r.URL.Path, "/index.m3u8"):
			fmt.Fprint(w, media(strings.Split(r.URL.Path, "/")[1]))
		default: // segment: /<variant>/seg<i>.ts
			segHits.Add(1)
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, strings.Repeat(r.URL.Path+";", 2000))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &segHits
}

func vodPlaylist(n int) func(string) string {
	return func(string) string {
		var b strings.Builder
		b.WriteString("#EXTM3U\n#EXT-X-TARGETDURATION:4\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "#EXTINF:4.0,\nseg%d.ts\n", i)
		}
		b.WriteString("#EXT-X-ENDLIST\n")
		return b.String()
	}
}

func wantJoined(variant string, n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString(strings.Repeat(fmt.Sprintf("/%s/seg%d.ts;", variant, i), 2000))
	}
	return b.Bytes()
}

func TestHLSPicksBestVariantAndJoins(t *testing.T) {
	t.Setenv("PATH", "") // no ffmpeg: keep .ts so the bytes can be compared
	srv, _ := hlsServer(t, 20, vodPlaylist(20), 0)
	dir := t.TempDir()
	d := &Download{Req: Request{URL: srv.URL + "/master.m3u8", Kind: "hls", Title: "My: Video?"}}
	if err := New(dir).Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(d.Path) != "My_ Video_.ts" {
		t.Fatalf("name %q", filepath.Base(d.Path))
	}
	got, _ := os.ReadFile(d.Path)
	if !bytes.Equal(got, wantJoined("high", 20)) {
		t.Fatal("joined output wrong (wrong variant or order)")
	}
	if d.Progress() != 100 {
		t.Fatalf("progress %.1f", d.Progress())
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 1 {
		t.Fatalf("leftover files: %v", files)
	}
}

func TestHLSPauseResume(t *testing.T) {
	t.Setenv("PATH", "")
	srv, hits := hlsServer(t, 30, vodPlaylist(30), 120*time.Millisecond)
	dir := t.TempDir()
	d := &Download{Req: Request{URL: srv.URL + "/master.m3u8", Kind: "hls"}}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(250*time.Millisecond, cancel)
	if err := New(dir).Run(ctx, d); !errors.Is(err, context.Canceled) || d.Status != "paused" {
		t.Fatalf("want paused, got %v / %s", err, d.Status)
	}
	paused := FindPaused(dir)
	if len(paused) != 1 {
		t.Fatalf("want 1 paused, got %d", len(paused))
	}
	before := hits.Load()
	r := paused[0]
	if err := New(dir).Run(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(r.Path)
	if !bytes.Equal(got, wantJoined("high", 30)) {
		t.Fatal("resumed output wrong")
	}
	refetched := int(hits.Load()-before) - 30
	t.Logf("paused after %d segment requests; resume fetched %d (%d of 30 were already on disk)", before, hits.Load()-before, -refetched)
	if hits.Load()-before >= 30 {
		t.Fatal("resume re-downloaded everything")
	}
}

func TestHLSFragmentedMP4(t *testing.T) {
	srv, _ := hlsServer(t, 3, func(string) string {
		return "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4,\nseg0.m4s\n#EXTINF:4,\nseg1.m4s\n#EXT-X-ENDLIST\n"
	}, 0)
	d := &Download{Req: Request{URL: srv.URL + "/master.m3u8", Kind: "hls", Title: "clip"}}
	if err := New(t.TempDir()).Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(d.Path)
	if filepath.Base(d.Path) != "clip.mp4" || !bytes.HasPrefix(got, []byte("/high/init.mp4;")) {
		t.Fatalf("want clip.mp4 starting with the init segment, got %s", filepath.Base(d.Path))
	}
}

func TestHLSRejectsEncryptedAndLive(t *testing.T) {
	enc, _ := hlsServer(t, 1, func(string) string {
		return "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXTINF:4,\nseg0.ts\n#EXT-X-ENDLIST\n"
	}, 0)
	live, _ := hlsServer(t, 1, func(string) string { return "#EXTM3U\n#EXTINF:4,\nseg0.ts\n" }, 0)
	for _, c := range []struct {
		url  string
		want error
	}{{enc.URL + "/master.m3u8", ErrEncrypted}, {live.URL + "/master.m3u8", ErrLive}} {
		dir := t.TempDir()
		d := &Download{Req: Request{URL: c.url, Kind: "hls"}}
		if err := New(dir).Run(context.Background(), d); !errors.Is(err, c.want) {
			t.Fatalf("want %v, got %v", c.want, err)
		}
		if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 0 {
			t.Fatalf("left files: %v", files)
		}
	}
}

func BenchmarkConcat(b *testing.B) {
	dir := b.TempDir()
	numParts := 100
	partSize := 64 * 1024
	data := bytes.Repeat([]byte("A"), partSize)
	for i := 0; i < numParts; i++ {
		path := filepath.Join(dir, fmt.Sprintf("part_%d", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatal(err)
		}
	}
	partFile := func(i int) string {
		return filepath.Join(dir, fmt.Sprintf("part_%d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dst := filepath.Join(dir, fmt.Sprintf("out_%d", i))
		if err := concat(dst, numParts, partFile); err != nil {
			b.Fatal(err)
		}
		os.Remove(dst)
	}
}

func BenchmarkParseMedia(b *testing.B) {
	u := "https://cdn.example.com/stream/video/index.m3u8?token=xyz123"
	text := vodPlaylist(1000)(u)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := parseMedia(u, text); err != nil {
			b.Fatal(err)
		}
	}
}
