package engine

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// Categories sort new downloads into subfolders by file type.
// Files of other types stay in the download folder itself.

var Categories = []string{"Video", "Music", "Images", "Documents", "E-books", "Compressed", "Disk images", "Programs", "Torrents", "Fonts"}

var categoryExt = map[string]string{}

func init() {
	for cat, exts := range map[string]string{
		"Video":       "mp4 m4v mkv avi mov webm flv wmv mpg mpeg ts m2ts 3gp ogv",
		"Music":       "mp3 flac m4a aac wav ogg opus wma alac aiff aif ape mka",
		"Images":      "jpg jpeg png gif webp bmp tif tiff svg heic heif avif ico psd raw cr2 nef dng",
		"Documents":   "pdf doc docx xls xlsx ppt pptx odt ods odp txt rtf csv md pages key numbers",
		"E-books":     "epub mobi azw azw3 fb2 djvu cbz cbr",
		"Compressed":  "zip rar 7z gz tgz tar bz2 tbz2 xz txz zst lz lzma cab arj lzh",
		"Disk images": "iso img vhd vhdx vmdk qcow2",
		"Programs":    "exe msi msix msixbundle appx appxbundle msu apk xapk aab dmg pkg deb rpm appimage flatpak snap jar",
		"Torrents":    "torrent",
		"Fonts":       "ttf otf woff woff2 ttc",
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
	if VideoPage(rawURL) {
		return true, "page"
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

// FolderFor is the folder a new download named name goes to by default.
func (e *Engine) FolderFor(name string) string { return e.targetDir(Request{}, name) }

// SetSortByType turns category subfolders on or off for new downloads.
func (e *Engine) SetSortByType(on bool) { e.sortByType.Store(on) }
