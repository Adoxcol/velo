package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
		if rel.Version != "0.2.0" || rel.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("release %+v", rel)
		}
		targetDir := filepath.Join(t.TempDir(), "subdir")
		path, err := Download(context.Background(), http.DefaultClient, rel, targetDir)
		if err != nil {
			t.Fatal(err)
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
