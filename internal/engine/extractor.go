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
	"runtime"
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

// IsPlaylistURL reports whether raw is a YouTube playlist page URL.
func IsPlaylistURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	on := func(domain string) bool { return h == domain || strings.HasSuffix(h, "."+domain) }
	return (on("youtube.com") || on("youtube-nocookie.com")) && u.Path == "/playlist" && u.Query().Get("list") != ""
}

// PlaylistEntry is one video inside an expanded playlist.
type PlaylistEntry struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ExpandPlaylist uses yt-dlp --flat-playlist to list the videos in a playlist.
func ExpandPlaylist(ctx context.Context, rawURL string) (string, []PlaylistEntry, error) {
	if !IsPlaylistURL(rawURL) {
		return "", nil, errors.New("not a supported playlist URL")
	}
	ytdlp, err := videoTool("yt-dlp")
	if err != nil {
		return "", nil, errors.New("playlist expansion needs yt-dlp; open Settings → Video Grabber to install it with one click")
	}
	args := []string{"--ignore-config", "--no-config-locations", "--no-exec", "--flat-playlist", "--dump-single-json", "--no-warnings"}
	if deno, err := videoTool("deno"); err == nil {
		args = append(args, "--js-runtimes", "deno:"+deno)
	}
	args = append(args, "--", rawURL)
	cmd := exec.CommandContext(ctx, ytdlp, args...)
	hideWindow(cmd)
	cmd.Cancel = func() error { return killProcessTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("could not read playlist: %w", err)
	}
	var data struct {
		Title   string `json:"title"`
		Entries []struct {
			ID         string `json:"id"`
			URL        string `json:"url"`
			WebpageURL string `json:"webpage_url"`
			Title      string `json:"title"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "", nil, fmt.Errorf("invalid playlist data: %w", err)
	}
	var entries []PlaylistEntry
	for _, e := range data.Entries {
		u := e.WebpageURL
		if u == "" && strings.HasPrefix(e.URL, "http") {
			u = e.URL
		}
		if u == "" && e.ID != "" {
			u = "https://www.youtube.com/watch?v=" + e.ID
		}
		if u == "" {
			continue
		}
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = u
		}
		entries = append(entries, PlaylistEntry{URL: u, Title: title})
	}
	if len(entries) == 0 {
		return "", nil, errors.New("no videos found in playlist")
	}
	return data.Title, entries, nil
}

func isRegularFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// VideoTool locates an external helper (ffmpeg, yt-dlp, deno) beside Velo,
// on the process PATH, in the live OS PATH registry, or in common package
// manager directories.
func VideoTool(name string) (string, error) {
	return videoTool(name)
}

func videoTool(name string) (string, error) {
	toolName := name
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(toolName), ".exe") {
		toolName += ".exe"
	}
	if app, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(app), toolName)
		if isRegularFile(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	// When PATH is not explicitly cleared (as in isolated unit tests), also
	// check live OS PATH settings and standard package-manager locations.
	if os.Getenv("PATH") != "" {
		if p := findPlatformTool(toolName); p != "" {
			return p, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func extractorExt(format string) string {
	if format == "mp4" {
		return ".mp4"
	}
	if format == "audio" {
		return ".mp3"
	}
	return ".mkv"
}

func maxFormatHeight(format string) int {
	switch format {
	case "1080p":
		return 1080
	case "720p":
		return 720
	case "480p":
		return 480
	}
	return 0
}

func extractorArgs(raw, dir, ffmpeg string) []string {
	return extractorArgsFormat(raw, dir, ffmpeg, "")
}

func extractorArgsFormat(raw, dir, ffmpeg, format string) []string {
	args := []string{"--ignore-config", "--no-config-locations", "--no-exec", "--no-playlist", "--max-downloads", "1", "--no-warnings", "--newline", "--progress", "--no-colors",
		"--no-overwrites", "--continue", "--no-mtime", "--match-filter", "!is_live & !is_upcoming & !has_drm"}
	switch format {
	case "mp4":
		args = append(args, "--format", "bv*[ext=mp4][vcodec^=avc]+ba[ext=m4a]/b[ext=mp4]/bv*+ba/b", "--merge-output-format", "mp4", "--recode-video", "mp4")
	case "audio":
		args = append(args, "--format", "ba/b", "--extract-audio", "--audio-format", "mp3", "--audio-quality", "0")
	case "1080p", "720p", "480p":
		h := strings.TrimSuffix(format, "p")
		args = append(args, "--format", fmt.Sprintf("bv*[height<=%s]+ba/b[height<=%s]/b", h, h), "--embed-subs", "--merge-output-format", "mkv", "--remux-video", "mkv")
	default:
		args = append(args, "--format", "bv*+ba/b", "--embed-subs", "--merge-output-format", "mkv", "--remux-video", "mkv")
	}
	return append(args, "--ffmpeg-location", ffmpeg, "--paths", dir, "--output", "video.%(ext)s",
		"--progress-template", "download:velo:%(progress)j", "--", raw)
}

// runPage delegates site-specific extraction to yt-dlp. Its files remain in the
// normal .parts directory so pause/restart and removal use Velo's existing lifecycle.
func (e *Engine) runPage(ctx context.Context, d *Download) error {
	u, ytdlp, ffmpeg, err := checkExtractorTools(d.Req.URL)
	if err != nil {
		return err
	}
	if d.Path == "" && d.Req.Filename == "" {
		if name, err := e.VideoFilename(ctx, d.Req); err == nil {
			d.mu.Lock()
			d.Req.Title = strings.TrimSuffix(name, extractorExt(d.Req.Format))
			d.mu.Unlock()
		}
	}
	work, err := e.prepareExtractorTarget(d)
	if err != nil {
		return err
	}
	args, cleanup, err := e.buildExtractorCmdArgs(u, d, ffmpeg, work)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	if err := withYouTubeCookieRetry(ctx, d.Req.URL, args, func(args []string) error {
		return runExtractorProcess(ctx, ytdlp, args, d)
	}); err != nil {
		return err
	}
	return finalizeExtractorOutput(d, work)
}

// VideoFilename resolves the selected video's metadata using the same credentials
// and proxy as the download. Browser tab titles may describe a search or feed.
func (e *Engine) VideoFilename(ctx context.Context, r Request) (string, error) {
	u, ytdlp, ffmpeg, err := checkExtractorTools(r.URL)
	if err != nil {
		return "", err
	}
	args, cleanup, err := e.buildExtractorCmdArgs(u, &Download{Req: r}, ffmpeg, os.TempDir())
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return "", err
	}
	// --max-downloads exits before --dump-single-json can print metadata.
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--max-downloads" {
			args = append(args[:i], args[i+2:]...)
			break
		}
	}
	args = append(append(args[:len(args)-2:len(args)-2], "--skip-download", "--dump-single-json"), "--", r.URL)
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var out []byte
	err = withYouTubeCookieRetry(ctx, r.URL, args, func(args []string) error {
		cmd := exec.CommandContext(ctx, ytdlp, args...)
		hideWindow(cmd)
		cmd.Cancel = func() error { return killProcessTree(cmd) }
		cmd.WaitDelay = 5 * time.Second
		var runErr error
		out, runErr = cmd.Output()
		return runErr
	})
	if err != nil {
		return "", err
	}
	var metadata struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &metadata); err != nil {
		return "", err
	}
	if strings.TrimSpace(metadata.Title) == "" {
		return "", errors.New("video metadata has no title")
	}
	return videoName(Request{Title: metadata.Title}) + extractorExt(r.Format), nil
}

func checkExtractorTools(rawURL string) (*url.URL, string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", "", errors.New("video page must be an HTTP or HTTPS URL")
	}
	ytdlp, err := videoTool("yt-dlp")
	if err != nil {
		return nil, "", "", errors.New("video page downloads need yt-dlp; open Settings → Video Grabber to install it with one click")
	}
	ffmpeg, err := videoTool("ffmpeg")
	if err != nil {
		return nil, "", "", errors.New("video page downloads need ffmpeg; open Settings → Video Grabber to install it with one click")
	}
	return u, ytdlp, ffmpeg, nil
}

func (e *Engine) prepareExtractorTarget(d *Download) (string, error) {
	ext := extractorExt(d.Req.Format)
	name := videoName(d.Req) + ext
	if d.Req.Filename != "" {
		name = SafeName(strings.TrimSuffix(d.Req.Filename, filepath.Ext(d.Req.Filename))) + ext
	}
	dir := e.targetDir(d.Req, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	d.mu.Lock()
	if d.Path == "" {
		d.Path = uniquePath(dir, name)
	}
	d.Total = -1
	d.mu.Unlock()
	if err := d.saveState(); err != nil {
		return "", err
	}
	work := d.Path + ".parts"
	if err := os.MkdirAll(work, 0755); err != nil {
		return "", err
	}
	return work, nil
}

func (e *Engine) buildExtractorCmdArgs(u *url.URL, d *Download, ffmpeg, work string) ([]string, func(), error) {
	args := extractorArgsFormat(d.Req.URL, work, ffmpeg, d.Req.Format)
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
		return nil, nil, err
	}
	proxyURL := ""
	if proxy != nil {
		proxyURL = proxy.String()
	}
	add("--proxy", proxyURL)

	var cleanup func()
	// Host-only cookies: do not turn browser cookies into a global HTTP header
	// that yt-dlp would forward to unrelated media CDNs.
	if d.Req.Cookies != "" {
		f, err := os.CreateTemp("", "velo-video-cookies-*.txt")
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { os.Remove(f.Name()) }
		_, err = f.WriteString(extractorCookies(u, d.Req.Cookies))
		closeErr := f.Close()
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		if closeErr != nil {
			cleanup()
			return nil, nil, closeErr
		}
		add("--cookies", f.Name())
	}
	return args, cleanup, nil
}

// YouTube can reject a browser session with HTTP 413 even for a public video.
// Retry that specific failure once without the cookie jar. Keep the original
// request intact, and never drop authentication for other errors or sites.
func withYouTubeCookieRetry(ctx context.Context, raw string, args []string, run func([]string) error) error {
	err := run(args)
	if err == nil || ctx.Err() != nil {
		return err
	}
	u, parseErr := url.Parse(raw)
	if parseErr != nil {
		return err
	}
	host := strings.ToLower(u.Hostname())
	if host != "youtu.be" && host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com") && host != "youtube-nocookie.com" && !strings.HasSuffix(host, ".youtube-nocookie.com") {
		return err
	}
	message := err.Error()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		message += string(exit.Stderr)
	}
	if !strings.Contains(message, "HTTP Error 413") && !strings.Contains(message, "HTTP Error 431") {
		return err
	}
	for i := 0; i+1 < len(args) && args[i] != "--"; i++ {
		if args[i] != "--cookies" {
			continue
		}
		clean := append([]string(nil), args[:i]...)
		clean = append(clean, args[i+2:]...)
		if retryErr := run(clean); retryErr != nil {
			return fmt.Errorf("YouTube rejected the browser cookies; retry without cookies failed: %w", retryErr)
		}
		return nil
	}
	return err
}

func runExtractorProcess(ctx context.Context, ytdlp string, args []string, d *Download) error {
	cmd := exec.CommandContext(ctx, ytdlp, args...)
	hideWindow(cmd)
	// CommandContext alone kills only yt-dlp, leaving ffmpeg writing the output.
	cmd.Cancel = func() error { return killProcessTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	log := &extractorLog{d: d}
	cmd.Stdout, cmd.Stderr = log, log
	d.active.Store(1)
	defer d.active.Store(0)
	err := cmd.Run()
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
	return nil
}

func finalizeExtractorOutput(d *Download, work string) error {
	finished := filepath.Join(work, "video"+extractorExt(d.Req.Format))
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
