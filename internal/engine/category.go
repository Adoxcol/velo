package engine

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// Categories sort new downloads into subfolders by file type.
// Files of other types stay in the download folder itself.

var Categories = []string{"Video", "Music", "Images", "Documents", "Compressed", "Programs"}

var categoryExt = map[string]string{}

func init() {
	for cat, exts := range map[string]string{
		"Video":      "mp4 m4v mkv avi mov webm flv wmv mpg mpeg ts 3gp",
		"Music":      "mp3 flac m4a aac wav ogg opus wma alac aiff",
		"Images":     "jpg jpeg png gif webp bmp tif tiff svg heic heif avif ico psd raw",
		"Documents":  "pdf doc docx xls xlsx ppt pptx odt ods odp txt rtf csv epub mobi md",
		"Compressed": "zip rar 7z gz tgz tar bz2 xz zst iso img cab",
		"Programs":   "exe msi msix appx apk dmg pkg deb rpm appimage",
	} {
		for _, e := range strings.Fields(exts) {
			categoryExt[e] = cat
		}
	}
}

// CategoryOf returns the category subfolder for a file name, or "".
func CategoryOf(name string) string {
	return categoryExt[strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")]
}

// Downloadable reports whether a URL points at a file type worth offering
// to download (a known category, or an HLS/DASH stream). Used for the
// clipboard monitor; returns the stream kind ("hls", "dash" or "").
func Downloadable(rawURL string) (ok bool, kind string) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false, ""
	}
	switch ext := strings.ToLower(path.Ext(u.Path)); ext {
	case ".m3u8":
		return true, "hls"
	case ".mpd":
		return true, "dash"
	default:
		return categoryExt[strings.TrimPrefix(ext, ".")] != "", ""
	}
}

// targetDir is the folder a new download named name should go to: the
// folder chosen for this download, else the download folder (plus the
// category subfolder when sorting is on).
func (e *Engine) targetDir(r Request, name string) string {
	if r.Dir != "" {
		return r.Dir
	}
	dir := e.Dir()
	if e.sortByType.Load() {
		if c := CategoryOf(name); c != "" {
			dir = filepath.Join(dir, c)
		}
	}
	return dir
}

// SetSortByType turns category subfolders on or off for new downloads.
func (e *Engine) SetSortByType(on bool) { e.sortByType.Store(on) }
