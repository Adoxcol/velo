package engine

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HLS (.m3u8) download: pick the best variant from a master playlist, fetch
// every media segment in parallel into <file>.parts/, then join them in
// order. Segments already on disk are skipped, so pause/resume works.
// Encrypted (AES-128, SAMPLE-AES, DRM) and live streams are not supported.

var (
	ErrEncrypted = errors.New("this stream is encrypted (DRM) and can't be downloaded")
	ErrLive      = errors.New("this is a live stream; only complete (on-demand) videos are supported")
)

const hlsWorkers = 6

type hlsPlaylist struct {
	init     string   // #EXT-X-MAP URI (fragmented MP4), "" for MPEG-TS
	segments []string // absolute segment URLs
}

func (e *Engine) fetchText(ctx context.Context, r Request, u string) (string, error) {
	rr := r
	rr.URL = u
	if hostOf(u) != hostOf(r.URL) { // logins and cookies stay with the site they are for
		rr.Cookies, rr.User, rr.Pass = "", "", ""
	}
	req, err := e.newReq(ctx, rr)
	if err != nil {
		return "", err
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playlist: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), err
}

// resolveURL resolves ref against a pre-parsed baseURL to avoid re-parsing
// the base URL string on every iteration of high-frequency segment loops.
func resolveURL(baseURL *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if baseURL == nil {
		return ref
	}
	r, err := baseURL.Parse(ref)
	if err != nil {
		return ref
	}
	return r.String()
}

func resolve(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return resolveURL(b, ref)
}

var attrRe = regexp.MustCompile(`([A-Z0-9-]+)=("[^"]*"|[^,]*)`)

func attrs(s string) map[string]string {
	m := map[string]string{}
	for _, kv := range attrRe.FindAllStringSubmatch(s, -1) {
		m[kv[1]] = strings.Trim(kv[2], `"`)
	}
	return m
}

// loadPlaylist fetches u; if it's a master playlist it follows the variant
// with the highest bandwidth (or the highest within r.Format's height cap).
func (e *Engine) loadPlaylist(ctx context.Context, r Request, u string) (*hlsPlaylist, error) {
	maxH := maxFormatHeight(r.Format)
	for depth := 0; depth < 3; depth++ {
		text, err := e.fetchText(ctx, r, u)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
			return nil, errors.New("not an HLS playlist")
		}
		if strings.Contains(text, "#EXT-X-STREAM-INF") {
			baseURL, _ := url.Parse(u)
			best, bestBW := "", int64(-1)
			fit, fitBW := "", int64(-1)
			sc := bufio.NewScanner(strings.NewReader(text))
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
					continue
				}
				at := attrs(line)
				bw, _ := strconv.ParseInt(at["BANDWIDTH"], 10, 64)
				h := 0
				if _, hs, ok := strings.Cut(at["RESOLUTION"], "x"); ok {
					h, _ = strconv.Atoi(hs)
				}
				for sc.Scan() { // the URI is the next non-comment line
					if next := strings.TrimSpace(sc.Text()); next != "" && !strings.HasPrefix(next, "#") {
						target := resolveURL(baseURL, next)
						if bw > bestBW {
							best, bestBW = target, bw
						}
						if maxH > 0 && h > 0 && h <= maxH && bw > fitBW {
							fit, fitBW = target, bw
						}
						break
					}
				}
			}
			if fit != "" {
				best = fit
			}
			if best == "" {
				return nil, errors.New("master playlist has no variants")
			}
			u = best
			continue
		}
		return parseMedia(u, text)
	}
	return nil, errors.New("playlist nesting too deep")
}

func parseMedia(u, text string) (*hlsPlaylist, error) {
	// Pre-parse the base URL once to avoid url.Parse overhead on every segment.
	baseURL, _ := url.Parse(u)
	p := &hlsPlaylist{}
	ended := false
	sc := bufio.NewScanner(strings.NewReader(text))
	// Use 4KB initial buffer with 1MB max limit to avoid allocating 1MB upfront per playlist parse.
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		if b[0] == '#' {
			if !bytes.HasPrefix(b, []byte("#EXT-X-")) {
				continue
			}
			line := string(b)
			switch {
			case strings.HasPrefix(line, "#EXT-X-KEY:"), strings.HasPrefix(line, "#EXT-X-SESSION-KEY:"):
				if m := attrs(line)["METHOD"]; m != "" && m != "NONE" {
					return nil, ErrEncrypted
				}
			case strings.HasPrefix(line, "#EXT-X-BYTERANGE"):
				return nil, errors.New("byte-range HLS playlists aren't supported yet")
			case strings.HasPrefix(line, "#EXT-X-MAP:"):
				p.init = resolveURL(baseURL, attrs(line)["URI"])
			case line == "#EXT-X-ENDLIST":
				ended = true
			}
			continue
		}
		p.segments = append(p.segments, resolveURL(baseURL, string(b)))
	}
	if !ended {
		return nil, ErrLive
	}
	if len(p.segments) == 0 {
		return nil, errors.New("playlist has no segments")
	}
	return p, nil
}

func (p *hlsPlaylist) parts() []string {
	if p.init != "" {
		return append([]string{p.init}, p.segments...)
	}
	return p.segments
}

func (p *hlsPlaylist) ext() string {
	if p.init != "" {
		return ".mp4"
	}
	s := strings.ToLower(path.Ext(strings.SplitN(p.segments[0], "?", 2)[0]))
	if s == ".m4s" || s == ".mp4" {
		return ".mp4"
	}
	return ".ts"
}

// videoName picks a file name (without extension) from the title or URL.
func videoName(r Request) string {
	name := r.Title
	if name == "" {
		if u, err := url.Parse(r.URL); err == nil {
			name = strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
		}
	}
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	name = safeName(name, "video")
	if name == "index" || name == "master" || name == "playlist" {
		name = "video"
	}
	return name
}

func (e *Engine) runHLS(ctx context.Context, d *Download) error {
	pl, err := e.loadPlaylist(ctx, d.Req, d.Req.URL)
	if err != nil {
		return err
	}
	return e.runStream(ctx, d, [][]string{pl.parts()}, pl.ext())
}

// runStream downloads one or more tracks of segments (HLS: one track;
// DASH: video + audio) into <file>.parts/, then joins them. Each track's
// first part is its init segment, if it has one.
func (e *Engine) runStream(ctx context.Context, d *Download, tracks [][]string, ext string) error {
	if len(tracks) > 1 {
		if _, err := videoTool("ffmpeg"); err != nil {
			return errors.New("ffmpeg is needed to merge this video's separate audio and video tracks; install it and add it to PATH")
		}
	}
	name := videoName(d.Req) + ext
	dir := e.targetDir(d.Req, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	d.mu.Lock()
	if d.Path == "" {
		d.Path = uniquePath(dir, name)
	}
	d.Total = -1
	d.mu.Unlock()
	if err := d.saveState(); err != nil {
		return err
	}
	partsDir := d.Path + ".parts"
	if err := os.MkdirAll(partsDir, 0o755); err != nil {
		return err
	}
	partFile := func(t, i int) string {
		if t == 0 {
			return filepath.Join(partsDir, fmt.Sprintf("%06d", i))
		}
		return filepath.Join(partsDir, fmt.Sprintf("%d_%06d", t, i))
	}

	total := 0
	for _, parts := range tracks {
		total += len(parts)
	}
	d.parts.Store(int64(total))
	d.partsDone.Store(0)
	d.partBytes.Store(0)
	d.Done.Store(0)

	type job struct{ t, i int }
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan job)
	var wg sync.WaitGroup
	var firstErr error
	var once sync.Once
	for w := 0; w < hlsWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.active.Add(1)
			defer d.active.Add(-1)
			for j := range jobs {
				n, err := e.fetchPart(ctx, d, tracks[j.t][j.i], partFile(j.t, j.i))
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					return
				}
				d.partBytes.Add(n)
				d.partsDone.Add(1)
			}
		}()
	}
feed:
	for t, parts := range tracks {
		for i := range parts {
			if fi, err := os.Stat(partFile(t, i)); err == nil {
				d.partsDone.Add(1)
				d.partBytes.Add(fi.Size())
				d.Done.Add(fi.Size())
			} else {
				select {
				case jobs <- job{t, i}:
				case <-ctx.Done():
					break feed
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	if firstErr != nil {
		return firstErr
	}
	if len(tracks) == 1 {
		return e.joinParts(d, len(tracks[0]), func(i int) string { return partFile(0, i) })
	}
	return e.muxTracks(d, tracks, partFile)
}

// fetchPart downloads one segment to dst, retrying; it writes to a temp
// file and renames, so a file at dst is always complete.
func (e *Engine) fetchPart(ctx context.Context, d *Download, u, dst string) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		rr := d.Req
		rr.URL = u
		if hostOf(u) != hostOf(d.Req.URL) { // logins and cookies stay with the site they are for
			rr.Cookies, rr.User, rr.Pass = "", "", ""
		}
		req, err := e.newReq(ctx, rr)
		if err != nil {
			return 0, err
		}
		resp, err := e.Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = &StatusError{resp.StatusCode, "segment: " + resp.Status}
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return 0, lastErr
		}
		tmp := dst + ".tmp"
		f, err := os.Create(tmp)
		if err != nil {
			resp.Body.Close()
			return 0, err
		}
		var n int64
		c := &counter{r: resp.Body, n: &d.Done, ctx: ctx, lim: e.Limit}
		n, err = io.Copy(f, c)
		resp.Body.Close()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			return n, os.Rename(tmp, dst)
		}
		os.Remove(tmp)
		d.Done.Add(-n) // this attempt's bytes will be fetched again
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		lastErr = err
	}
	return 0, fmt.Errorf("segment failed: %w", lastErr)
}

// concat writes parts 0..n-1 one after another into dst.
// Uses a 256KB bufio.Writer and a single pre-allocated 64KB buffer to avoid allocating
// a new 32KB buffer on every segment file copy and to reduce write syscalls.
func concat(dst string, n int, partFile func(int) string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(out, 256<<10)
	buf := make([]byte, 64<<10)
	for i := 0; i < n; i++ {
		in, err := os.Open(partFile(i))
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.CopyBuffer(bw, in, buf)
		in.Close()
		if err != nil {
			out.Close()
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// muxTracks joins each track, then merges them with ffmpeg into one MP4.
func (e *Engine) muxTracks(d *Download, tracks [][]string, partFile func(t, i int) string) error {
	ff, err := videoTool("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg is needed to merge audio and video")
	}
	args := []string{"-v", "error", "-y"}
	for t := range tracks {
		joined := filepath.Join(d.Path+".parts", fmt.Sprintf("track%d.mp4", t))
		if err := concat(joined, len(tracks[t]), func(i int) string { return partFile(t, i) }); err != nil {
			return err
		}
		args = append(args, "-i", joined)
	}
	for t := range tracks {
		args = append(args, "-map", strconv.Itoa(t))
	}
	args = append(args, "-c", "copy", "-f", "mp4", d.Path+".part")
	cmd := exec.Command(ff, args...)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merging audio and video failed: %s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(d.Path+".part", d.Path); err != nil {
		return err
	}
	os.RemoveAll(d.Path + ".parts")
	os.Remove(statePath(d.Path))
	d.mu.Lock()
	if fi, err := os.Stat(d.Path); err == nil {
		d.Total = fi.Size()
	}
	d.mu.Unlock()
	return nil
}

// joinParts concatenates the parts into the final file; MPEG-TS output is
// remuxed to MP4 when ffmpeg is available.
func (e *Engine) joinParts(d *Download, n int, partFile func(int) string) error {
	if err := concat(d.Path+".part", n, partFile); err != nil {
		return err
	}
	final := d.Path
	if strings.HasSuffix(final, ".ts") {
		if ff, err := videoTool("ffmpeg"); err == nil {
			mp4 := uniquePath(filepath.Dir(final), strings.TrimSuffix(filepath.Base(final), ".ts")+".mp4")
			cmd := exec.Command(ff, "-v", "error", "-y", "-f", "mpegts", "-i", d.Path+".part",
				"-c", "copy", "-bsf:a", "aac_adtstoasc", mp4)
			hideWindow(cmd)
			if cmd.Run() == nil {
				os.Remove(d.Path + ".part")
				final = mp4
			} else {
				os.Remove(mp4)
			}
		}
	}
	if final == d.Path {
		if err := os.Rename(d.Path+".part", final); err != nil {
			return err
		}
	}
	os.RemoveAll(d.Path + ".parts")
	os.Remove(statePath(d.Path))
	d.mu.Lock()
	d.Path = final
	if fi, err := os.Stat(final); err == nil {
		d.Total = fi.Size()
	}
	d.mu.Unlock()
	return nil
}
