package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVideoPage(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://www.youtube.com/watch?v=abc&list=xyz": true,
		"https://youtu.be/abc":                         true,
		"https://www.youtube.com/shorts/abc":           true,
		"https://www.youtube-nocookie.com/embed/abc":   true,
		"https://x.com/person/status/123/video/1":      true,
		"https://www.facebook.com/watch/?v=123":        true,
		"https://www.facebook.com/person/videos/123/":  true,
		"https://www.facebook.com/reel/123":            true,
		"https://youtube.com/playlist?list=abc":        false,
		"https://x.com/home":                           false,
		"https://facebook.com/":                        false,
		"https://youtube.com.evil.test/watch?v=abc":    false,
		"file://youtube.com/watch?v=abc":               false,
	} {
		if got := VideoPage(raw); got != want {
			t.Errorf("VideoPage(%q) = %v", raw, got)
		}
		if want {
			if ok, kind := Downloadable(raw); !ok || kind != "page" {
				t.Errorf("not routed to extractor: %s", raw)
			}
		}
	}
}

func TestExtractorArgsSecurity(t *testing.T) {
	args := extractorArgs("https://example.com/video", "/tmp/dir", "/usr/bin/ffmpeg")
	hasArg := func(target string) bool {
		for _, arg := range args {
			if arg == target {
				return true
			}
		}
		return false
	}
	for _, flag := range []string{"--ignore-config", "--no-config-locations", "--no-exec"} {
		if !hasArg(flag) {
			t.Errorf("extractorArgs missing security flag %q", flag)
		}
	}
}

func TestExtractorCookies(t *testing.T) {
	u, _ := url.Parse("https://www.facebook.com/watch?v=1")
	got := extractorCookies(u, "session=secret=value; bad=one\ntwo; other=ok")
	if strings.Contains(got, "bad") || !strings.Contains(got, "www.facebook.com\tFALSE\t/\tTRUE\t0\tsession\tsecret=value") {
		t.Fatal(got)
	}
}

func TestExtractorProgressAndResume(t *testing.T) {
	d := &Download{Req: Request{URL: "https://youtu.be/abc", Kind: "page", Cookies: "secret"}, Path: filepath.Join(t.TempDir(), "video.mkv"), Total: -1}
	l := &extractorLog{d: d}
	l.Write([]byte("velo:{\"downloaded_bytes\":20,"))
	l.Write([]byte("\"total_bytes\":100}\nERROR: test failure\n"))
	if d.Done.Load() != 20 || d.Progress() != 20 || l.lastError != "ERROR: test failure" {
		t.Fatal("incorrect progress", d.Done.Load(), d.Progress(), l.lastError)
	}
	if err := d.saveState(); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(statePath(d.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !restored.isStream() || restored.Ranged || restored.Req.Cookies != "" || restored.Path != d.Path {
		t.Fatal("incorrect page resume state")
	}
}

// Opt-in real subprocess smoke test, entirely against a local HTTP server.
func TestExtractorLocalIntegration(t *testing.T) {
	if os.Getenv("VELO_TEST_EXTRACTOR") != "1" {
		t.Skip("set VELO_TEST_EXTRACTOR=1 with yt-dlp and ffmpeg installed")
	}
	ffmpeg, err := videoTool("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", fixture)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	e := New(filepath.Join(dir, "output"))
	e.SetProxy("none")
	d := &Download{Req: Request{URL: srv.URL + "/fixture.mp4", Kind: "page", Title: "Local video"}}
	if err := e.Run(ctx, d); err != nil {
		t.Fatal(err)
	}
	if state, _ := d.State(); state != "done" {
		t.Fatal(state)
	}
	p, total := d.Info()
	if filepath.Ext(p) != ".mkv" || total <= 0 {
		t.Fatal(p, total)
	}
	if _, err := os.Stat(statePath(p)); !os.IsNotExist(err) {
		t.Fatal("state file remains")
	}
}

func TestVideoToolFallbackDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeTool := "velo-fake-tool-test"
	fileName := fakeTool
	if filepath.Separator == '\\' {
		fileName += ".exe"
	}
	want := filepath.Join(binDir, fileName)
	if err := os.WriteFile(want, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Non-empty PATH that does not contain binDir: fallback discovery finds it.
	t.Setenv("PATH", t.TempDir())
	got, err := videoTool(fakeTool)
	if err != nil || got != want {
		t.Fatalf("videoTool(%q) = %q, %v; want %q", fakeTool, got, err, want)
	}

	// Explicitly empty PATH (used by isolated tests): fallback is skipped.
	t.Setenv("PATH", "")
	if _, err := videoTool(fakeTool); err == nil {
		t.Fatal("expected error when PATH is explicitly empty")
	}
}

func TestExtractorArgsFormatsAndRemainingGaps(t *testing.T) {
	if extractorExt("audio") != ".mp3" || extractorExt("1080p") != ".mkv" || extractorExt("") != ".mkv" {
		t.Fatal("unexpected extractorExt")
	}
	audioArgs := strings.Join(extractorArgsFormat("https://example.com/v", "/tmp", "ffmpeg", "audio"), " ")
	if !strings.Contains(audioArgs, "--extract-audio") || !strings.Contains(audioArgs, "--audio-format mp3") {
		t.Fatalf("audio args missing extract flags: %s", audioArgs)
	}
	hdArgs := strings.Join(extractorArgsFormat("https://example.com/v", "/tmp", "ffmpeg", "720p"), " ")
	if !strings.Contains(hdArgs, "height<=720") {
		t.Fatalf("720p args missing height cap: %s", hdArgs)
	}

	d := &Download{
		Total:  1000,
		Ranged: true,
		segs: []*Segment{
			{Pos: 100, End: 499},
			{Pos: 600, End: 999},
		},
	}
	d.Done.Store(200)
	gaps := d.RemainingGaps()
	if len(gaps) != 2 || gaps[0][0] != 10 || gaps[0][1] != 40 || gaps[1][0] != 60 || gaps[1][1] != 40 {
		t.Fatalf("unexpected RemainingGaps: %+v", gaps)
	}
}
