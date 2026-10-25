package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	updateDir := filepath.Join(os.TempDir(), "Velo-update")
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
