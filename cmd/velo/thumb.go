//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Adoxcol/velo/internal/engine"
)

var (
	gdi32           = syscall.NewLazyDLL("gdi32.dll")
	pSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	pDestroyIcon    = user32.NewProc("DestroyIcon")
	pGetIconInfo    = user32.NewProc("GetIconInfo")
	pGetDC          = user32.NewProc("GetDC")
	pReleaseDC      = user32.NewProc("ReleaseDC")
	pGetObjectW     = gdi32.NewProc("GetObjectW")
	pGetDIBits      = gdi32.NewProc("GetDIBits")
	pDeleteObject   = gdi32.NewProc("DeleteObject")
	thumbCache      sync.Map // key -> thumbResult
	shellIconMu     sync.Mutex
)

// youtubeThumbURL returns the thumbnail URL for a YouTube video/Shorts URL, or "".
func youtubeThumbURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	var id string
	switch {
	case h == "youtu.be" || strings.HasSuffix(h, ".youtu.be"):
		id = strings.Trim(u.Path, "/")
	case h == "youtube.com" || strings.HasSuffix(h, ".youtube.com") || h == "youtube-nocookie.com" || strings.HasSuffix(h, ".youtube-nocookie.com"):
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if parts := strings.Split(strings.Trim(u.Path, "/"), "/"); len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
			id = parts[1]
		}
	}
	if len(id) < 6 || strings.ContainsAny(id, "/?&\"'<>\\") {
		return ""
	}
	return "https://i.ytimg.com/vi/" + id + "/mqdefault.jpg"
}

// itemPreview returns a thumbnail or native file icon for download #id.
func (a *app) itemPreview(id int) thumbResult {
	it := a.get(id)
	if it == nil {
		return thumbResult{}
	}
	d := it.d
	status, _ := d.State()
	fullPath, _ := d.Info()
	name := filepath.Base(fullPath)
	if fullPath == "" {
		name = d.Req.Filename
		if name == "" {
			name = d.Req.Title
		}
	}
	ext := strings.ToLower(filepath.Ext(name))

	// 1. Completed file on disk: try image preview, video frame, or embedded .exe/.ico icon.
	if status == "done" && fullPath != "" {
		if st, err := os.Stat(fullPath); err == nil && !st.IsDir() {
			cacheKey := fmt.Sprintf("done:%s:%d:%d", fullPath, st.Size(), st.ModTime().UnixNano())
			if v, ok := thumbCache.Load(cacheKey); ok {
				return v.(thumbResult)
			}
			diskPath := a.diskThumbPath(cacheKey)
			if diskPath != "" {
				if b, err := os.ReadFile(diskPath); err == nil && len(b) > 0 {
					res := thumbResult{Src: string(b), Mode: "cover"}
					thumbCache.Store(cacheKey, res)
					return res
				}
			}
			switch ext {
			case ".png", ".jpg", ".jpeg", ".gif":
				if src := imageFileThumb(fullPath, st.Size()); src != "" {
					res := thumbResult{Src: src, Mode: "cover"}
					thumbCache.Store(cacheKey, res)
					a.saveDiskThumb(diskPath, src)
					return res
				}
			case ".mp4", ".mkv", ".webm", ".mov", ".avi", ".ts", ".m4v":
				if src := videoFileThumb(fullPath); src != "" {
					res := thumbResult{Src: src, Mode: "cover"}
					thumbCache.Store(cacheKey, res)
					a.saveDiskThumb(diskPath, src)
					return res
				}
			}
			// If it was a YouTube video and ffmpeg couldn't extract a frame, keep the YouTube thumbnail.
			if yt := youtubeThumbURL(d.Req.URL); yt != "" {
				res := thumbResult{Src: yt, Mode: "cover"}
				thumbCache.Store(cacheKey, res)
				return res
			}
			// Extract the file's real Windows icon (including embedded .exe icons).
			if src := extractShellIcon(fullPath, false); src != "" {
				res := thumbResult{Src: src, Mode: "icon"}
				thumbCache.Store(cacheKey, res)
				return res
			}
		}
	}

	return a.urlPreview(d.Req.URL, name, d.Req.Kind)
}

// urlPreview returns a preview for a URL + filename before the file is on disk.
func (a *app) urlPreview(rawURL, name, kind string) thumbResult {
	if yt := youtubeThumbURL(rawURL); yt != "" {
		return thumbResult{Src: yt, Mode: "cover"}
	}
	ext := strings.ToLower(filepath.Ext(name))
	if kind == "" {
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg":
			if strings.HasPrefix(strings.ToLower(rawURL), "https://") || strings.HasPrefix(strings.ToLower(rawURL), "http://") {
				return thumbResult{Src: rawURL, Mode: "cover"}
			}
		}
	}
	if ext != "" {
		return a.fileTypeIcon(name)
	}
	return thumbResult{}
}

func (a *app) diskThumbPath(key string) string {
	if a.home == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(a.home, "thumbs", hex.EncodeToString(sum[:12])+".txt")
}

func (a *app) saveDiskThumb(path, dataURI string) {
	if path == "" || dataURI == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
		_ = os.WriteFile(path, []byte(dataURI), 0o600)
	}
}

// fileTypeIcon returns the Windows shell icon for a filename/extension.
func (a *app) fileTypeIcon(name string) thumbResult {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return thumbResult{}
	}
	cacheKey := "ext:" + ext
	if v, ok := thumbCache.Load(cacheKey); ok {
		return v.(thumbResult)
	}
	if src := extractShellIcon("file"+ext, true); src != "" {
		res := thumbResult{Src: src, Mode: "icon"}
		thumbCache.Store(cacheKey, res)
		return res
	}
	return thumbResult{}
}

func imageFileThumb(path string, size int64) string {
	if size <= 0 || size > 20<<20 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return ""
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}
	// Center-crop and downsample to 96x96 so the data URI is tiny (~4-8 KB).
	const target = 96
	dst := image.NewNRGBA(image.Rect(0, 0, target, target))
	side := min(w, h)
	offX := b.Min.X + (w-side)/2
	offY := b.Min.Y + (h-side)/2
	for y := 0; y < target; y++ {
		sy := offY + y*side/target
		for x := 0; x < target; x++ {
			sx := offX + x*side/target
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func videoFileThumb(path string) string {
	ff, err := engine.VideoTool("ffmpeg")
	if err != nil || ff == "" {
		return ""
	}
	for _, seek := range []string{"00:00:02", "00:00:00"} {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		cmd := exec.CommandContext(ctx, ff,
			"-v", "error",
			"-ss", seek,
			"-i", path,
			"-frames:v", "1",
			"-vf", "scale=160:-1",
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		out, err := cmd.Output()
		cancel()
		if err == nil && len(out) > 0 {
			return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out)
		}
	}
	return ""
}

type shFileInfoW struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

type winIconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type winBitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// extractShellIcon retrieves the Windows Shell icon for path (or a virtual
// filename when useFileAttrs is true) and encodes it as a data:image/png URI.
func extractShellIcon(path string, useFileAttrs bool) string {
	shellIconMu.Lock()
	defer shellIconMu.Unlock()

	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	const (
		shgfiIcon           = 0x000000100
		shgfiLargeIcon      = 0x000000000
		shgfiUseFileAttrs   = 0x000000010
		fileAttributeNormal = 0x00000080
	)
	flags := uint32(shgfiIcon | shgfiLargeIcon)
	attrs := uint32(0)
	if useFileAttrs {
		flags |= shgfiUseFileAttrs
		attrs = fileAttributeNormal
	}
	var shfi shFileInfoW
	r, _, _ := pSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(attrs),
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		uintptr(flags),
	)
	if r == 0 || shfi.hIcon == 0 {
		return ""
	}
	defer pDestroyIcon.Call(shfi.hIcon)

	var ii winIconInfo
	if ok, _, _ := pGetIconInfo.Call(shfi.hIcon, uintptr(unsafe.Pointer(&ii))); ok == 0 {
		return ""
	}
	if ii.hbmMask != 0 {
		defer pDeleteObject.Call(ii.hbmMask)
	}
	if ii.hbmColor == 0 {
		return ""
	}
	defer pDeleteObject.Call(ii.hbmColor)

	var bm winBitmap
	if n, _, _ := pGetObjectW.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); n == 0 || bm.bmWidth <= 0 || bm.bmHeight <= 0 {
		return ""
	}
	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if w > 256 || h > 256 {
		return ""
	}

	hdc, _, _ := pGetDC.Call(0)
	if hdc == 0 {
		return ""
	}
	defer pReleaseDC.Call(0, hdc)

	hdr := bitmapInfoHeader{
		biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:    int32(w),
		biHeight:   -int32(h), // top-down DIB
		biPlanes:   1,
		biBitCount: 32,
	}
	raw := make([]byte, w*h*4)
	if lines, _, _ := pGetDIBits.Call(
		hdc,
		ii.hbmColor,
		0,
		uintptr(h),
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(unsafe.Pointer(&hdr)),
		0,
	); lines == 0 {
		return ""
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	hasAlpha := false
	for i := 0; i < len(raw); i += 4 {
		if raw[i+3] != 0 {
			hasAlpha = true
			break
		}
	}
	for i := 0; i < len(raw); i += 4 {
		b, g, r, a := raw[i], raw[i+1], raw[i+2], raw[i+3]
		if !hasAlpha {
			a = 255
		}
		img.Pix[i] = r
		img.Pix[i+1] = g
		img.Pix[i+2] = b
		img.Pix[i+3] = a
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
