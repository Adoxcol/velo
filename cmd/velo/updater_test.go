package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/update"
	"github.com/Adoxcol/velo/internal/version"
)

func TestUpdaterCheckLifecycle(t *testing.T) {
	oldVer := version.Version
	version.Version = "0.8.0"
	t.Cleanup(func() { version.Version = oldVer })

	body := []byte("fake setup content")
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			assets := fmt.Sprintf(`{"name":"Velo-Setup-99.0.0.exe","browser_download_url":"%s/setup","digest":"sha256:%s"}`, "http://"+r.Host, hexSum)
			fmt.Fprintf(w, `{"tag_name":"v99.0.0","html_url":"https://example.com/rel","body":"notes","assets":[%s]}`, assets)
		case "/setup":
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	oldAPI := update.API
	update.API = srv.URL + "/latest"
	t.Cleanup(func() { update.API = oldAPI })

	a := testApp(t, defaultSettings())

	// Create pre-existing update file so update.Download is instant.
	updateDir := tempUpdateDir(t)
	os.MkdirAll(updateDir, 0o700)
	setupFile := filepath.Join(updateDir, "Velo-Setup-99.0.0.exe")
	os.WriteFile(setupFile, body, 0o600)

	a.upd.check(a)

	info := a.upd.info()
	if info.Checking {
		t.Fatal("expected u.checking to be false after check completes")
	}
	if info.Available != "99.0.0" {
		t.Fatalf("expected Available = 99.0.0, got %s", info.Available)
	}
	if !info.Ready {
		t.Fatalf("expected Ready = true, got %v", info.Ready)
	}
	if info.Status != "Velo 99.0.0 is ready to install." {
		t.Fatalf("unexpected status: %s", info.Status)
	}
}

func TestUpdaterUpToDateStatus(t *testing.T) {
	oldVer := version.Version
	version.Version = "0.8.0"
	t.Cleanup(func() { version.Version = oldVer })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assets := `{"name":"Velo-Setup-0.8.0.exe","browser_download_url":"http://example.com/setup","digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}`
		fmt.Fprintf(w, `{"tag_name":"v0.8.0","html_url":"https://example.com/rel","body":"notes","assets":[%s]}`, assets)
	}))
	t.Cleanup(srv.Close)

	oldAPI := update.API
	update.API = srv.URL + "/latest"
	t.Cleanup(func() { update.API = oldAPI })

	a := testApp(t, defaultSettings())
	a.upd.check(a)

	info := a.upd.info()
	if info.Checking {
		t.Fatal("expected u.checking to be false")
	}
	if info.Status != "Velo is up to date." {
		t.Fatalf("expected 'Velo is up to date.', got %q", info.Status)
	}
}

func TestUpdaterDeduplication(t *testing.T) {
	oldVer := version.Version
	version.Version = "0.8.0"
	t.Cleanup(func() { version.Version = oldVer })

	var requestCount atomic.Int32
	unblock := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		<-unblock
		assets := `{"name":"Velo-Setup-0.8.0.exe","browser_download_url":"http://example.com/setup","digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}`
		fmt.Fprintf(w, `{"tag_name":"v0.8.0","html_url":"https://example.com/rel","body":"notes","assets":[%s]}`, assets)
	}))
	t.Cleanup(srv.Close)

	oldAPI := update.API
	update.API = srv.URL + "/latest"
	t.Cleanup(func() { update.API = oldAPI })

	a := testApp(t, defaultSettings())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.upd.check(a)
	}()

	// Wait for the first check to start and hit the server.
	for requestCount.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	info := a.upd.info()
	if !info.Checking {
		t.Fatal("expected u.busy (Checking) to be true while check is in progress")
	}

	// Launch a second concurrent check; it should return immediately because u.busy is true.
	a.upd.check(a)

	close(unblock)
	wg.Wait()

	if count := requestCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 server request due to deduplication, got %d", count)
	}
}

func TestUpdaterDownloadStatus(t *testing.T) {
	oldVer := version.Version
	version.Version = "0.8.0"
	t.Cleanup(func() { version.Version = oldVer })

	body := []byte("fake setup content")
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])

	downloadBlock := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			assets := fmt.Sprintf(`{"name":"Velo-Setup-99.0.0.exe","browser_download_url":"%s/setup","digest":"sha256:%s"}`, "http://"+r.Host, hexSum)
			fmt.Fprintf(w, `{"tag_name":"v99.0.0","html_url":"https://example.com/rel","body":"notes","assets":[%s]}`, assets)
		case "/setup":
			<-downloadBlock
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	oldAPI := update.API
	update.API = srv.URL + "/latest"
	t.Cleanup(func() { update.API = oldAPI })

	// Ensure no leftover file from previous tests.
	updateDir := tempUpdateDir(t)
	os.RemoveAll(updateDir)

	a := testApp(t, defaultSettings())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.upd.check(a)
	}()

	// Poll until status changes to Downloading...
	var info updateInfo
	for i := 0; i < 100; i++ {
		info = a.upd.info()
		if info.Status == "Downloading Velo 99.0.0…" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if info.Status != "Downloading Velo 99.0.0…" {
		t.Fatalf("expected status 'Downloading Velo 99.0.0…', got %q", info.Status)
	}
	if !info.Checking {
		t.Fatal("expected Checking to be true while downloading")
	}

	close(downloadBlock)
	wg.Wait()

	info = a.upd.info()
	if info.Status != "Velo 99.0.0 is ready to install." {
		t.Fatalf("expected status 'Velo 99.0.0 is ready to install.', got %q", info.Status)
	}
}

func TestUpdaterNetworkTimeout(t *testing.T) {
	oldVer := version.Version
	version.Version = "0.8.0"
	t.Cleanup(func() { version.Version = oldVer })

	// Point to an unreachable IP address that times out / refuses connection.
	oldAPI := update.API
	update.API = "http://127.0.0.1:0/latest"
	t.Cleanup(func() { update.API = oldAPI })

	a := testApp(t, defaultSettings())
	a.upd.check(a)

	info := a.upd.info()
	if info.Checking {
		t.Fatal("expected Checking to be false after timeout failure")
	}
	if !testing.Verbose() && len(info.Status) == 0 {
		t.Fatal("expected non-empty status on network error")
	}
}

// tempUpdateDir points the updater at a throwaway folder for this test.
func tempUpdateDir(t *testing.T) string {
	dir := t.TempDir()
	old := updateDir
	updateDir = func() string { return dir }
	t.Cleanup(func() { updateDir = old })
	return dir
}
