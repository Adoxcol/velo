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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	SumURL  string `json:"sum_url,omitempty"`
	Page    string `json:"page"` // release notes page
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
	for _, a := range gr.Assets {
		switch a.Name {
		case want:
			rel.URL = a.URL
			if d, ok := strings.CutPrefix(a.Digest, "sha256:"); ok {
				rel.SHA256 = strings.ToLower(d)
			}
		case want + ".sha256":
			rel.SumURL = a.URL
		}
	}
	if rel.URL == "" {
		return nil, fmt.Errorf("release %s has no %s", gr.Tag, want)
	}
	if rel.SHA256 == "" && rel.SumURL == "" {
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

// Download saves the installer into dir and checks its SHA-256. A slow or
// unsteady connection is fine: each attempt continues the partial file from
// where the last one stopped, and an attempt only gives up when no data
// arrives for StallTimeout.
func Download(ctx context.Context, client *http.Client, rel *Release, dir string) (string, error) {
	// Two downloads at once (an automatic check and Check for updates)
	// would append to the same .part file and fail the checksum.
	downloadMu.Lock()
	defer downloadMu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "Velo-Setup-"+rel.Version+".exe")

	ensureChecksum := func() error {
		if rel.SHA256 == "" && rel.SumURL != "" {
			sum, err := fetchSum(ctx, client, rel.SumURL)
			if err != nil {
				return err
			}
			rel.SHA256 = sum
		}
		if len(rel.SHA256) != 64 {
			return fmt.Errorf("release %s has invalid checksum", rel.Version)
		}
		return nil
	}

	if sum, err := fileSHA256(dst); err == nil {
		if err := ensureChecksum(); err == nil && sum == rel.SHA256 {
			return dst, nil
		}
	}

	if err := ensureChecksum(); err != nil {
		return "", err
	}

	tmp := dst + ".part"
	var err error
	for attempt := 0; attempt < Attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 3 * time.Second):
			}
		}
		if err = downloadPart(ctx, client, rel.URL, tmp); err == nil {
			break
		}
		if ctx.Err() != nil || errors.Is(err, errTooLarge) {
			break
		}
	}
	if err != nil {
		return "", err
	}
	sum, err := fileSHA256(tmp)
	if err == nil && sum != rel.SHA256 {
		err = errors.New("the downloaded update failed its checksum check, so it wasn't installed")
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

var downloadMu sync.Mutex

// Attempts and StallTimeout bound an update download.
var (
	Attempts     = 6
	StallTimeout = 60 * time.Second
)

var errTooLarge = errors.New("update is unexpectedly large")

// downloadPart fetches the rest of url into tmp, continuing what is there.
func downloadPart(ctx context.Context, client *http.Client, url, tmp string) error {
	have := int64(0)
	if fi, err := os.Stat(tmp); err == nil {
		have = fi.Size()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Velo-updater")
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flag := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0:
		return nil // already complete; the checksum decides
	case resp.StatusCode == http.StatusOK:
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC // server sent the whole file
		have = 0
	default:
		return fmt.Errorf("downloading the update: %s", resp.Status)
	}
	f, err := os.OpenFile(tmp, flag, 0o600)
	if err != nil {
		return err
	}
	// Cancel the request if no data arrives for StallTimeout.
	var stalled atomic.Bool
	stall := time.AfterFunc(StallTimeout, func() { stalled.Store(true); cancel() })
	defer stall.Stop()
	body := &stallReader{r: resp.Body, t: stall}
	n, err := io.Copy(f, io.LimitReader(body, MaxSize+1-have))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && have+n > MaxSize {
		os.Remove(tmp)
		return errTooLarge
	}
	if err != nil && stalled.Load() {
		return errors.New("the download stalled; Velo will try again later")
	}
	return err
}

// stallReader restarts the stall timer whenever data arrives.
type stallReader struct {
	r io.Reader
	t *time.Timer
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.t.Reset(StallTimeout)
	}
	return n, err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Install starts the verified installer silently; it closes Velo, replaces
// it and starts the new version.
func Install(path string) error {
	return runInstaller(path)
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
