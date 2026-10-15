// Package update checks GitHub Releases for a newer Velo, downloads its
// installer and verifies it before running it.
//
// A release is tagged vX.Y.Z and has an asset named Velo-Setup-X.Y.Z.exe.
// The download must match a SHA-256 checksum: GitHub's own asset digest when
// available, otherwise a Velo-Setup-X.Y.Z.exe.sha256 asset next to it.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// API is the "latest release" endpoint (changed in tests).
var API = "https://api.github.com/repos/Adoxcol/velo/releases/latest"

// MaxSize is the largest installer accepted.
const MaxSize = 300 << 20

// ErrNoRelease means the repository has no published release (or is private).
var ErrNoRelease = errors.New("no published release found")

type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`    // installer download
	SHA256  string `json:"sha256"` // expected checksum, hex
	Page    string `json:"page"`   // release notes page
	Notes   string `json:"notes"`
}

type ghRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Page       string `json:"html_url"`
	Body       string `json:"body"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"` // "sha256:<hex>" (newer GitHub API)
	} `json:"assets"`
}

// Latest returns the newest published release.
func Latest(ctx context.Context, client *http.Client) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", API, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Velo-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release check: %s", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return nil, err
	}
	if gr.Draft || gr.Prerelease {
		return nil, ErrNoRelease
	}
	ver := strings.TrimPrefix(gr.Tag, "v")
	if _, ok := parse(ver); !ok {
		return nil, fmt.Errorf("release tag %q isn't a version", gr.Tag)
	}
	rel := &Release{Version: ver, Page: gr.Page, Notes: gr.Body}
	want := "Velo-Setup-" + ver + ".exe"
	var sumURL string
	for _, a := range gr.Assets {
		switch a.Name {
		case want:
			rel.URL = a.URL
			if d, ok := strings.CutPrefix(a.Digest, "sha256:"); ok {
				rel.SHA256 = strings.ToLower(d)
			}
		case want + ".sha256":
			sumURL = a.URL
		}
	}
	if rel.URL == "" {
		return nil, fmt.Errorf("release %s has no %s", gr.Tag, want)
	}
	if rel.SHA256 == "" && sumURL != "" {
		if rel.SHA256, err = fetchSum(ctx, client, sumURL); err != nil {
			return nil, err
		}
	}
	if len(rel.SHA256) != 64 {
		return nil, fmt.Errorf("release %s has no checksum for its installer", gr.Tag)
	}
	return rel, nil
}

func fetchSum(ctx context.Context, client *http.Client, u string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "Velo-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(b)) // "hex  filename" or just "hex"
	if len(f) == 0 {
		return "", errors.New("empty checksum file")
	}
	return strings.ToLower(f[0]), nil
}

// Download saves the installer into dir and checks its SHA-256.
func Download(ctx context.Context, client *http.Client, rel *Release, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rel.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Velo-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading the update: %s", resp.Status)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "Velo-Setup-"+rel.Version+".exe")
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, MaxSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxSize {
		err = errors.New("update is unexpectedly large")
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != rel.SHA256 {
		err = errors.New("the downloaded update failed its checksum check, so it wasn't installed")
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

// Install starts the verified installer silently; it closes Velo, replaces
// it and starts the new version.
func Install(path string) error {
	cmd := exec.Command(path, "/S", "/update")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

// Newer reports whether version a is newer than b ("dev" is never newer
// and never updated).
func Newer(a, b string) bool {
	va, ok1 := parse(a)
	vb, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
