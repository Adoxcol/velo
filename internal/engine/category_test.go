package engine

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCategoryOf(t *testing.T) {
	for name, want := range map[string]string{
		"movie.MKV": "Video", "song.mp3": "Music", "report.pdf": "Documents",
		"backup.tar.gz": "Compressed", "setup.exe": "Programs", "notes": "", "photo.JPG": "Images", "page.html": "",
	} {
		if got := CategoryOf(name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestSortByType(t *testing.T) {
	data := randomData(t, 1<<20)
	srv, _ := testServer(t, data, 0, 0)
	dir := t.TempDir()
	e := New(dir)
	e.SetSortByType(true)
	d := &Download{Req: Request{URL: srv.URL + "/file.bin", Filename: "archive.zip"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Compressed", "archive.zip"); d.Path != want {
		t.Fatalf("saved to %s, want %s", d.Path, want)
	}
	check(t, d, data)

	e.SetSortByType(false)
	d2 := &Download{Req: Request{URL: srv.URL + "/file.bin", Filename: "archive.zip"}}
	if err := e.Run(context.Background(), d2); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "archive.zip"); d2.Path != want {
		t.Fatalf("with sorting off, saved to %s", d2.Path)
	}
}

func TestSortByTypeVideoStream(t *testing.T) {
	t.Setenv("PATH", "")
	srv, _ := hlsServer(t, 3, vodPlaylist(3), 0)
	dir := t.TempDir()
	e := New(dir)
	e.SetSortByType(true)
	d := &Download{Req: Request{URL: srv.URL + "/master.m3u8", Kind: "hls", Title: "clip"}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Video", "clip.ts"); d.Path != want {
		t.Fatalf("saved to %s, want %s", d.Path, want)
	}
}

func TestDownloadable(t *testing.T) {
	for u, want := range map[string]string{
		"https://x.com/a/file.zip":         "file",
		"https://x.com/a/file.zip?token=1": "file",
		"http://x.com/v/master.m3u8":       "hls",
		"https://x.com/v/manifest.mpd?x=y": "dash",
		"https://x.com/page.html":          "",
		"https://x.com/":                   "",
		"ftp://x.com/file.zip":             "",
		"file:///C:/a.zip":                 "",
		"not a url":                        "",
	} {
		ok, kind := Downloadable(u)
		got := ""
		if ok {
			got = "file"
			if kind != "" {
				got = kind
			}
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", u, got, want)
		}
	}
}

func TestProbeAndOverrides(t *testing.T) {
	data := randomData(t, 3<<20)
	srv, _ := testServer(t, data, 0, 0)
	e := New(t.TempDir())
	info, err := e.Probe(context.Background(), Request{URL: srv.URL + "/report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "report.pdf" || info.Size != int64(len(data)) || !info.Resumable || info.Category != "Documents" {
		t.Fatalf("probe: %+v", info)
	}
	// A chosen folder and connection count override the defaults.
	custom := filepath.Join(t.TempDir(), "chosen")
	e.SetSortByType(true)
	d := &Download{Req: Request{URL: srv.URL + "/report.pdf", Filename: "renamed.pdf", Dir: custom, Segments: 3}}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if d.Path != filepath.Join(custom, "renamed.pdf") {
		t.Fatalf("saved to %s", d.Path)
	}
	check(t, d, data)
}
