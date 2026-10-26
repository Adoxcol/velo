//go:build windows

package main

import (
	_ "embed"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"

	"github.com/Adoxcol/velo/internal/i18n"
	"github.com/Adoxcol/velo/internal/ipc"
)

// Progress windows, like IDM's: after Start download in the download dialog,
// a small window per download shows its progress, speed, time left and the
// parts downloading at once, with Pause/Resume, Hide and Cancel. It closes
// when the download finishes (the "Download complete" window takes over).

//go:embed progress.html
var progressHTML string

var (
	progMu   sync.Mutex
	progWins = map[int]webview2.WebView{}
	progOpen = map[int]bool{} // being created
)

const progW, progH = 560, 272

// progressRow is the main list's row for one download, or nil.
func (a *app) progressRow(id int) *row {
	for _, r := range a.rows() {
		if r.ID == id {
			r := r
			return &r
		}
	}
	return nil
}

// showProgress opens the progress window for download id (or brings it up).
func showProgress(a *app, id int) {
	progMu.Lock()
	defer progMu.Unlock()
	if w := progWins[id]; w != nil {
		w.Dispatch(func() { showWindow.Call(uintptr(w.Window()), 9) }) // SW_RESTORE
		return
	}
	if progOpen[id] {
		return
	}
	progOpen[id] = true
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath: filepath.Join(ipc.Dir(), "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: "Velo — Downloading", Width: progW, Height: progH, Center: true,
			},
		})
		progMu.Lock()
		delete(progOpen, id)
		if w == nil {
			progMu.Unlock()
			log.Print("could not create the progress window")
			return
		}
		hwnd := uintptr(w.Window())
		setWindowIcon(hwnd)
		const wsThickFrame, wsMaximizeBox = 0x40000, 0x10000
		style, _, _ := pGetWindowLongPtr.Call(hwnd, index(gwlStyle))
		pSetWindowLongPtr.Call(hwnd, index(gwlStyle), style&^(wsThickFrame|wsMaximizeBox))
		pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x1|0x2|0x4|swpFrameChanged) // NOSIZE|NOMOVE|NOZORDER

		w.Bind("progressID", func() int { return id })
		w.Bind("progressInfo", func() *row { return a.progressRow(id) })
		w.Bind("pauseDownload", func() error { return a.pause(id) })
		w.Bind("resumeDownload", func() error { return a.resume(id) })
		w.Bind("cancelDownload", func() error { return a.remove(id) })
		var closed atomic.Bool
		bindWindowAsync(w, &closed, "fileTypeIcon", func(name string) (thumbResult, error) { return a.fileTypeIcon(name), nil })
		w.Bind("getSettings", func() Settings { return a.getSettings() })
		w.Bind("openMain", func() { go a.show() })
		w.Bind("closeProgress", func() { go closeProgress(id) })
		w.SetHtml(i18n.Inject(progressHTML, a.lang()))
		progWins[id] = w
		progMu.Unlock()
		w.Run()
		closed.Store(true)
		progMu.Lock()
		delete(progWins, id)
		progMu.Unlock()
	}()
}

func closeProgress(id int) {
	progMu.Lock()
	w := progWins[id]
	progMu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Destroy() })
	}
}
