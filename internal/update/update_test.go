package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.1.1", "0.1.0", true}, {"0.2.0", "0.1.9", true}, {"1.0.0", "0.9.9", true},
		{"0.1.0", "0.1.0", false}, {"0.1.0", "0.1.1", false}, {"0.10.0", "0.9.0", true},
		{"0.1.1", "dev", false}, {"x", "0.1.0", false}, {"v0.2.0", "0.1.0", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// fakeGitHub serves a release whose installer is body; digest controls the
// checksum GitHub reports ("" = use a .sha256 asset instead).
func fakeGitHub(t *testing.T, body []byte, digest string) *httptest.Server {
	sum := sha256.Sum256(body)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			assets := fmt.Sprintf(`{"name":"Velo-Setup-0.2.0.exe","browser_download_url":"%s/setup","digest":"%s"}`, srv.URL, digest)
			if digest == "" {
				assets += fmt.Sprintf(`,{"name":"Velo-Setup-0.2.0.exe.sha256","browser_download_url":"%s/sum"}`, srv.URL)
			}
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","html_url":"https://example/rel","body":"notes","assets":[%s]}`, assets)
		case "/setup":
			w.Write(body)
		case "/sum":
			fmt.Fprintf(w, "%s  Velo-Setup-0.2.0.exe\n", hex.EncodeToString(sum[:]))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	API = srv.URL + "/latest"
	return srv
}

func TestCheckAndDownload(t *testing.T) {
	body := []byte("pretend installer")
	sum := sha256.Sum256(body)
	for _, digest := range []string{"sha256:" + hex.EncodeToString(sum[:]), ""} {
		fakeGitHub(t, body, digest)
		rel, err := Latest(context.Background(), http.DefaultClient)
		if err != nil {
			t.Fatal(err)
		}
		if rel.Version != "0.2.0" {
			t.Fatalf("release version %+v", rel)
		}
		if digest != "" && rel.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("release SHA256 %+v", rel)
		}
		targetDir := filepath.Join(t.TempDir(), "subdir")
		path, err := Download(context.Background(), http.DefaultClient, rel, targetDir)
		if err != nil {
			t.Fatal(err)
		}
		if rel.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("release SHA256 after download %+v", rel)
		}
		if b, _ := os.ReadFile(path); string(b) != string(body) {
			t.Fatal("wrong installer contents")
		}
		fi, err := os.Stat(targetDir)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if perm := fi.Mode().Perm(); perm != 0o700 {
				t.Fatalf("expected directory permissions 0700, got %o", perm)
			}
		}
	}
}

func TestTamperedDownloadIsRejected(t *testing.T) {
	fakeGitHub(t, []byte("tampered installer"), "sha256:"+hex.EncodeToString(make([]byte, 32)))
	rel, err := Latest(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := Download(context.Background(), http.DefaultClient, rel, dir); err == nil {
		t.Fatal("a download with the wrong checksum was accepted")
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatalf("left files behind: %v", files)
	}
}

func TestNoRelease(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler()) // private repo or no releases
	defer srv.Close()
	API = srv.URL + "/latest"
	if _, err := Latest(context.Background(), http.DefaultClient); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("got %v", err)
	}
}

// A connection that drops halfway, then one that stalls, still ends with a
// complete, verified installer: each attempt continues the partial file.
func TestDownloadResumesAfterDropAndStall(t *testing.T) {
	body := make([]byte, 300_000)
	for i := range body {
		body[i] = byte(i * 7)
	}
	sum := sha256.Sum256(body)
	oldStall := StallTimeout
	StallTimeout = 1 * time.Second
	defer func() { StallTimeout = oldStall }()

	var calls int
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		ranges = append(ranges, r.Header.Get("Range"))
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			fmt.Sscanf(rg, "bytes=%d-", &start)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.Header().Set("Content-Length", strconv.Itoa(len(body)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		switch calls {
		case 1: // drop the connection after 100 KB
			w.Write(body[start:100_000])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		case 2: // send 100 KB more, then go silent
			w.Write(body[start:200_000])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			w.Write(body[start:])
		}
	}))
	defer srv.Close()

	rel := &Release{Version: "9.9.9", URL: srv.URL, SHA256: hex.EncodeToString(sum[:])}
	path, err := Download(context.Background(), srv.Client(), rel, t.TempDir())
	if err != nil {
		t.Fatalf("download failed: %v (requests: %q)", err, ranges)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, body) {
		t.Fatal("downloaded installer differs from the original")
	}
	if calls != 3 || ranges[0] != "" || ranges[1] != "bytes=100000-" || ranges[2] != "bytes=200000-" {
		t.Fatalf("expected 3 requests continuing at 0, 100000, 200000; got %d: %q", calls, ranges)
	}
}

func TestDownloadReusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("already downloaded installer")
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])

	dst := filepath.Join(dir, "Velo-Setup-0.3.0.exe")
	if err := os.WriteFile(dst, body, 0o600); err != nil {
		t.Fatal(err)
	}

	// URL points to an invalid host; if Download attempts HTTP GET, it will fail.
	rel := &Release{
		Version: "0.3.0",
		URL:     "http://127.0.0.1:0/unreachable",
		SHA256:  hexSum,
	}

	gotPath, err := Download(context.Background(), http.DefaultClient, rel, dir)
	if err != nil {
		t.Fatalf("expected Download to reuse existing file, got err = %v", err)
	}
	if gotPath != dst {
		t.Fatalf("expected path %s, got %s", dst, gotPath)
	}
}

func TestConcurrentDownloadsBothVerify(t *testing.T) {
	body := bytes.Repeat([]byte("velo"), 256<<10)
	sum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "setup.exe", time.Time{}, bytes.NewReader(body))
	}))
	defer srv.Close()
	dir := t.TempDir()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			rel := &Release{Version: "9.9.9", URL: srv.URL, SHA256: hex.EncodeToString(sum[:])}
			_, err := Download(context.Background(), srv.Client(), rel, dir)
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// When the API refuses the check (rate limit), the release is found from the
// web page's redirect and verified with its .sha256 asset.
func TestRateLimitedCheckFallsBackToWeb(t *testing.T) {
	body := []byte("installer")
	sum := sha256.Sum256(body)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			http.Error(w, "rate limited", http.StatusForbidden)
		case "/releases/latest":
			http.Redirect(w, r, srv.URL+"/releases/tag/v9.9.9", http.StatusFound)
		case "/releases/download/v9.9.9/Velo-Setup-9.9.9.exe":
			w.Write(body)
		case "/releases/download/v9.9.9/Velo-Setup-9.9.9.exe.sha256":
			fmt.Fprintf(w, "%s *Velo-Setup-9.9.9.exe\n", hex.EncodeToString(sum[:]))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldAPI, oldWeb := API, Web
	API, Web = srv.URL+"/api/latest", srv.URL+"/releases"
	t.Cleanup(func() { API, Web = oldAPI, oldWeb })

	rel, err := Latest(context.Background(), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "9.9.9" {
		t.Fatalf("version %q", rel.Version)
	}
	if _, err := Download(context.Background(), srv.Client(), rel, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestInstallMock(t *testing.T) {
	orig := runInstaller
	defer func() { runInstaller = orig }()

	var calledWith string
	var runCount int
	runInstaller = func(path string) error {
		calledWith = path
		runCount++
		return nil
	}

	testPath := filepath.Join(t.TempDir(), "Velo-Setup-1.0.0.exe")
	if err := Install(testPath); err != nil {
		t.Fatalf("Install(%q) returned unexpected error: %v", testPath, err)
	}
	if runCount != 1 {
		t.Fatalf("expected runInstaller to be called once, got %d", runCount)
	}
	if calledWith != testPath {
		t.Fatalf("expected runInstaller called with %q, got %q", testPath, calledWith)
	}

	// Test error propagation
	expectedErr := errors.New("failed to launch installer")
	runInstaller = func(path string) error {
		return expectedErr
	}
	if err := Install(testPath); !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestInstallDefaultRealExecution(t *testing.T) {
	// Tests calling Install with defaultRunInstaller on a nonexistent file
	nonexistent := filepath.Join(t.TempDir(), "nonexistent_installer.exe")
	if err := Install(nonexistent); err == nil {
		t.Fatal("expected error executing nonexistent installer, got nil")
	}

	if runtime.GOOS != "windows" {
		// On non-Windows platforms, create a mock executable script to verify
		// that defaultRunInstaller runs the process with expected arguments.
		recordFile := filepath.Join(t.TempDir(), "args.txt")
		scriptPath := filepath.Join(t.TempDir(), "fake_installer.sh")
		scriptContent := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %q\n", recordFile)
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := Install(scriptPath); err != nil {
			t.Fatalf("Install on valid executable failed: %v", err)
		}
		// Give the started process a moment to run and write args.txt
		var recorded []byte
		for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
			if b, err := os.ReadFile(recordFile); err == nil && len(b) > 0 {
				recorded = b
				break
			}
		}
		if string(bytes.TrimSpace(recorded)) != "/S /update" {
			t.Fatalf("expected arguments '/S /update', got %q", string(recorded))
		}
	}
}

// The silent-install arguments are built separately per platform, so check
// them on every platform, Windows included.
func TestInstallerArguments(t *testing.T) {
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })

	var name string
	var args []string
	missing := filepath.Join(t.TempDir(), "missing.exe")
	execCommand = func(n string, a ...string) *exec.Cmd {
		name, args = n, a
		return orig(missing) // fails to start, so nothing actually runs
	}
	if err := defaultRunInstaller("Velo-Setup.exe"); err == nil {
		t.Fatal("expected the missing command to fail to start")
	}
	if name != "Velo-Setup.exe" || strings.Join(args, " ") != "/S /update" {
		t.Fatalf("ran %q with %q, want Velo-Setup.exe with /S /update", name, args)
	}
}
