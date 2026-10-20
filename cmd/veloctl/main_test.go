package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetCommand(t *testing.T) {
	if os.Getenv("VELO_CLI_TEST_CHILD") == "1" {
		os.Args = []string{"veloctl", "get", os.Getenv("VELO_CLI_TEST_URL"), os.Getenv("VELO_CLI_TEST_DIR")}
		main()
		return
	}
	payload := bytes.Repeat([]byte("portable-download\n"), 10000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="portable.bin"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestGetCommand$")
	cmd.Env = append(os.Environ(), "VELO_CLI_TEST_CHILD=1", "VELO_CLI_TEST_URL="+srv.URL+"/file", "VELO_CLI_TEST_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CLI: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "portable.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download differs: %v", err)
	}
}

func TestStreamKind(t *testing.T) {
	for raw, want := range map[string]string{
		"https://youtube.com/watch?v=test":         "page",
		"https://example.com/video.m3u8?token=123": "hls",
		"https://example.com/video.mpd":            "dash",
		"https://example.com/file.zip":             "",
	} {
		if got := streamKind(raw); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

func TestParseFlagsAndURLFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "links.txt")
	content := "# comment\n\nhttps://example.com/a.zip\nhttps://youtube.com/watch?v=abc\n"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	urls, dir, format := parseFlags([]string{"-format", "720p", "-i", tmp, "/tmp/out"})
	if len(urls) != 2 || urls[0] != "https://example.com/a.zip" || dir != "/tmp/out" || format != "720p" {
		t.Fatalf("unexpected parseFlags result: urls=%v dir=%q format=%q", urls, dir, format)
	}
}
