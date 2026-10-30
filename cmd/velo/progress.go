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
		// No UnlockOSThread: the thread exits with the goroutine, taking any
		// hidden windows (shell, COM, WebView2) with it instead of leaving them
		// on a pooled thread that never reads messages and stalls input.
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

		var closed atomic.Bool
		w.Bind("progressID", func() int { return id })
		bindWindowAsync(w, &closed, "progressInfo", func(_ any) (*row, error) { return a.progressRow(id), nil })
		bindWindowAsync(w, &closed, "pauseDownload", func(_ any) (bool, error) { return true, a.pause(id) })
		bindWindowAsync(w, &closed, "resumeDownload", func(_ any) (bool, error) { return true, a.resume(id) })
		bindWindowAsync(w, &closed, "cancelDownload", func(_ any) (bool, error) { return true, a.remove(id) })
		bindWindowAsync(w, &closed, "fileTypeIcon", func(name string) (thumbResult, error) { return a.fileTypeIcon(name), nil })
		bindWindowAsync(w, &closed, "getSettings", func(_ any) (Settings, error) { return a.getSettings(), nil })
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
