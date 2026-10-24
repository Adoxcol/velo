//go:build windows

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
)

var (
	vtMu         sync.Mutex
	vtInstalling bool
	vtStep       string
	vtErr        string
)

func (a *app) videoToolsStatus() videoToolsState {
	specs := []struct{ name, role string }{
		{"yt-dlp", "YouTube, X & Facebook page extractor"},
		{"ffmpeg", "Audio/video merger & video thumbnails"},
		{"deno", "JavaScript runtime for YouTube"},
	}
	var tools []videoToolInfo
	for _, s := range specs {
		p, err := engine.VideoTool(s.name)
		tools = append(tools, videoToolInfo{
			Name:      s.name,
			Role:      s.role,
			Installed: err == nil && p != "",
			Path:      p,
		})
	}
	vtMu.Lock()
	defer vtMu.Unlock()
	return videoToolsState{
		Tools:      tools,
		Installing: vtInstalling,
		Step:       vtStep,
		Error:      vtErr,
	}
}

func (a *app) installVideoTools() error {
	vtMu.Lock()
	if vtInstalling {
		vtMu.Unlock()
		return nil
	}
	vtInstalling = true
	vtStep = "Preparing…"
	vtErr = ""
	vtMu.Unlock()

	go func() {
		err := runVideoToolsInstall(func(step string) {
			vtMu.Lock()
			vtStep = step
			vtMu.Unlock()
		})
		vtMu.Lock()
		vtInstalling = false
		if err != nil {
			vtErr = err.Error()
			vtStep = ""
		} else {
			vtErr = ""
			vtStep = "All video tools are installed and up to date."
		}
		vtMu.Unlock()
	}()
	return nil
}

func videoToolsTargetDir() (string, error) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		testFile := filepath.Join(dir, ".velo-write-test")
		if err := os.WriteFile(testFile, []byte("ok"), 0o644); err == nil {
			os.Remove(testFile)
			return dir, nil
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dir := filepath.Join(local, "Programs", "Velo")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("could not find a writable folder for video tools")
}

func runVideoToolsInstall(progress func(string)) error {
	dir, err := videoToolsTargetDir()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}

	// 1. Install or update yt-dlp.exe
	ytdlpDest := filepath.Join(dir, "yt-dlp.exe")
	if existing, err := engine.VideoTool("yt-dlp"); err == nil && strings.EqualFold(filepath.Clean(existing), filepath.Clean(ytdlpDest)) {
		progress("Updating yt-dlp…")
		cmd := exec.Command(existing, "-U")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := cmd.Run(); err != nil {
			progress("Downloading latest yt-dlp.exe…")
			if err := downloadFileAtomic(client, "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe", ytdlpDest); err != nil {
				return fmt.Errorf("yt-dlp: %w", err)
			}
		}
	} else {
		progress("Downloading yt-dlp.exe…")
		if err := downloadFileAtomic(client, "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe", ytdlpDest); err != nil {
			return fmt.Errorf("yt-dlp: %w", err)
		}
	}

	// 2. Install deno.exe if missing
	if _, err := engine.VideoTool("deno"); err != nil {
		progress("Downloading Deno runtime (deno.exe)…")
		if err := downloadZipEntry(client,
			"https://github.com/denoland/deno/releases/latest/download/deno-x86_64-pc-windows-msvc.zip",
			"deno.exe",
			filepath.Join(dir, "deno.exe"),
		); err != nil {
			return fmt.Errorf("deno: %w", err)
		}
	}

	// 3. Install ffmpeg.exe if missing
	if _, err := engine.VideoTool("ffmpeg"); err != nil {
		progress("Downloading ffmpeg (this may take a moment)…")
		if err := downloadZipEntry(client,
			"https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip",
			"ffmpeg.exe",
			filepath.Join(dir, "ffmpeg.exe"),
		); err != nil {
			return fmt.Errorf("ffmpeg: %w", err)
		}
	}

	return nil
}

func downloadFileAtomic(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dest)
}

func downloadZipEntry(client *http.Client, url, baseName, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	tmpZip, err := os.CreateTemp("", "velo-tool-*.zip")
	if err != nil {
		return err
	}
	zipPath := tmpZip.Name()
	defer os.Remove(zipPath)
	if _, err := io.Copy(tmpZip, resp.Body); err != nil {
		tmpZip.Close()
		return err
	}
	if err := tmpZip.Close(); err != nil {
		return err
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	want := strings.ToLower(baseName)
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if strings.ToLower(filepath.Base(zf.Name)) == want {
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			tmp := dest + ".tmp"
			out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				rc.Close()
				return err
			}
			_, copyErr := io.Copy(out, rc)
			rc.Close()
			closeErr := out.Close()
			if copyErr != nil {
				os.Remove(tmp)
				return copyErr
			}
			if closeErr != nil {
				os.Remove(tmp)
				return closeErr
			}
			return os.Rename(tmp, dest)
		}
	}
	return fmt.Errorf("%s not found in archive", baseName)
}
