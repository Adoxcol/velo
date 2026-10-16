package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VideoPage identifies individual videos, never a site's home feed or playlist.
func VideoPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	on := func(domain string) bool { return h == domain || strings.HasSuffix(h, "."+domain) }
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if on("youtu.be") {
		return len(p) == 1 && p[0] != ""
	}
	if on("youtube.com") || on("youtube-nocookie.com") {
		return (u.Path == "/watch" && u.Query().Get("v") != "") || (len(p) == 2 && (p[0] == "shorts" || p[0] == "embed" || p[0] == "live") && p[1] != "")
	}
	if on("x.com") || on("twitter.com") {
		return len(p) >= 3 && p[1] == "status" && p[2] != ""
	}
	if on("facebook.com") {
		return ((u.Path == "/watch/" || u.Path == "/watch" || u.Path == "/video.php") && u.Query().Get("v") != "") || (len(p) == 2 && p[0] == "reel" && p[1] != "") || (len(p) >= 3 && p[1] == "videos" && p[2] != "")
	}
	return false
}

func videoTool(name string) (string, error) {
	if app, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(app), name+".exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return exec.LookPath(name)
}

func extractorArgs(raw, dir, ffmpeg string) []string {
	return []string{"--ignore-config", "--no-playlist", "--max-downloads", "1", "--no-warnings", "--newline", "--progress", "--no-colors",
		"--no-overwrites", "--continue", "--no-mtime", "--match-filter", "!is_live & !is_upcoming & !has_drm",
		"--format", "bv*+ba/b", "--merge-output-format", "mkv", "--remux-video", "mkv",
		"--ffmpeg-location", ffmpeg, "--paths", dir, "--output", "video.%(ext)s",
		"--progress-template", "download:velo:%(progress)j", "--", raw}
}

// runPage delegates site-specific extraction to yt-dlp. Its files remain in the
// normal .parts directory so pause/restart and removal use Velo's existing lifecycle.
func (e *Engine) runPage(ctx context.Context, d *Download) error {
	u, err := url.Parse(d.Req.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("video page must be an HTTP or HTTPS URL")
	}
	ytdlp, err := videoTool("yt-dlp")
	if err != nil {
		return errors.New("video page downloads need yt-dlp; install yt-dlp and add it to PATH, or place yt-dlp.exe beside Velo")
	}
	ffmpeg, err := videoTool("ffmpeg")
	if err != nil {
		return errors.New("video page downloads need ffmpeg to combine audio and video; add it to PATH or place ffmpeg.exe beside Velo")
	}
	name := videoName(d.Req) + ".mkv"
	if d.Req.Filename != "" {
		name = SafeName(strings.TrimSuffix(d.Req.Filename, filepath.Ext(d.Req.Filename))) + ".mkv"
	}
	dir := e.targetDir(d.Req, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
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
	work := d.Path + ".parts"
	if err := os.MkdirAll(work, 0755); err != nil {
		return err
	}
	args := extractorArgs(d.Req.URL, work, ffmpeg)
	// Insert options before the end-of-options marker; URLs never become flags.
	add := func(values ...string) {
		args = append(append(args[:len(args)-2:len(args)-2], values...), "--", d.Req.URL)
	}
	if d.Req.UserAgent != "" {
		add("--user-agent", d.Req.UserAgent)
	}
	if d.Req.Referrer != "" {
		add("--referer", d.Req.Referrer)
	}
	if deno, err := videoTool("deno"); err == nil {
		add("--js-runtimes", "deno:"+deno)
	}
	if e.Limit != nil && e.Limit.Rate() > 0 {
		add("--limit-rate", strconv.FormatInt(e.Limit.Rate(), 10))
	}
	req, _ := http.NewRequest("GET", d.Req.URL, nil)
	proxy, err := e.proxyFor(req)
	if err != nil {
		return err
	}
	proxyURL := ""
	if proxy != nil {
		proxyURL = proxy.String()
	}
	add("--proxy", proxyURL)
	// Host-only cookies: do not turn browser cookies into a global HTTP header
	// that yt-dlp would forward to unrelated media CDNs.
	if d.Req.Cookies != "" {
		f, err := os.CreateTemp("", "velo-video-cookies-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = f.WriteString(extractorCookies(u, d.Req.Cookies))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		add("--cookies", f.Name())
	}
	cmd := exec.CommandContext(ctx, ytdlp, args...)
	hideWindow(cmd)
	// CommandContext alone kills only yt-dlp, leaving ffmpeg writing the output.
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		hideWindow(kill)
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
	log := &extractorLog{d: d}
	cmd.Stdout, cmd.Stderr = log, log
	d.active.Store(1)
	defer d.active.Store(0)
	err = cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// --max-downloads reports code 101 after successfully writing the one video.
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 101) {
		if log.lastError == "" {
			log.lastError = err.Error()
		}
		return fmt.Errorf("video extraction failed: %s (update yt-dlp and its JavaScript runtime if this site changed)", log.lastError)
	}
	finished := filepath.Join(work, "video.mkv")
	st, err := os.Stat(finished)
	if err != nil || st.Size() == 0 {
		return errors.New("video extractor did not produce a complete video")
	}
	if _, err := os.Stat(d.Path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("output file already exists; choose a different file name")
	}
	if err := os.Rename(finished, d.Path); err != nil {
		return err
	}
	d.Done.Store(st.Size())
	d.mu.Lock()
	d.Total = st.Size()
	d.mu.Unlock()
	os.RemoveAll(work)
	os.Remove(statePath(d.Path))
	return nil
}

func extractorCookies(u *url.URL, raw string) string {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(c), "=")
		if !ok || name == "" || strings.ContainsAny(name+value, "\t\r\n") {
			continue
		}
		fmt.Fprintf(&b, "%s\tFALSE\t/\t%t\t0\t%s\t%s\n", u.Hostname(), u.Scheme == "https", name, value)
	}
	return strings.ReplaceAll(strings.ReplaceAll(b.String(), "\ttrue\t", "\tTRUE\t"), "\tfalse\t", "\tFALSE\t")
}

// os/exec serializes writes when stdout and stderr share the same writer.
type extractorLog struct {
	d                  *Download
	pending, lastError string
}

func (l *extractorLog) Write(p []byte) (int, error) {
	l.pending += string(p)
	for {
		line, rest, ok := strings.Cut(l.pending, "\n")
		if !ok {
			break
		}
		l.pending = rest
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "ERROR:") {
			l.lastError = line
		}
		if strings.HasPrefix(line, "velo:") {
			var progress struct {
				Downloaded int64 `json:"downloaded_bytes"`
				Total      int64 `json:"total_bytes"`
				Estimate   int64 `json:"total_bytes_estimate"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "velo:")), &progress) == nil {
				l.d.Done.Store(progress.Downloaded)
				l.d.mu.Lock()
				l.d.Total = max(progress.Total, progress.Estimate)
				l.d.mu.Unlock()
			}
		}
	}
	if len(l.pending) > 65536 {
		l.pending = ""
	}
	return len(p), nil
}
