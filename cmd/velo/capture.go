package main

import (
	_ "embed"
	"log"
	"path/filepath"
	"runtime"
	"sync"

	webview2 "github.com/jchv/go-webview2"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/i18n"
	"github.com/Adoxcol/velo/internal/ipc"
)

// The download dialog: a small window of its own, like IDM's "Download
// File Info", that opens when the browser hands Velo a download. It shows
// the pending downloads one at a time and closes when none are left; the
// main window stays closed.

//go:embed capture.html
var captureHTML string

var (
	capMu      sync.Mutex
	capWin     webview2.WebView
	capOpening bool
)

const capW, capH = 700, 292

// showCapture opens the download dialog, or brings it to the front and
// tells it a new download is waiting.
func showCapture(a *app) {
	capMu.Lock()
	defer capMu.Unlock()
	if capWin != nil {
		w := capWin
		w.Dispatch(func() {
			w.Eval("window.refreshPending && refreshPending()")
			hwnd := uintptr(w.Window())
			showWindow.Call(hwnd, 9) // SW_RESTORE
			setForeground.Call(hwnd)
		})
		return
	}
	if capOpening {
		return
	}
	capOpening = true
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath:  filepath.Join(ipc.Dir(), "webview"),
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title: "Velo — Download file", Width: capW, Height: capH, Center: true,
			},
		})
		capMu.Lock()
		capOpening = false
		if w == nil {
			capMu.Unlock()
			log.Print("could not create the download dialog")
			return
		}
		hwnd := uintptr(w.Window())
		setWindowIcon(hwnd)
		// A fixed-size dialog: no maximize box, no resizing.
		const wsThickFrame, wsMaximizeBox = 0x40000, 0x10000
		style, _, _ := pGetWindowLongPtr.Call(hwnd, index(gwlStyle))
		pSetWindowLongPtr.Call(hwnd, index(gwlStyle), style&^(wsThickFrame|wsMaximizeBox))
		// Always on top, like a browser download prompt.
		pSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, 0x1|0x2|swpFrameChanged) // SWP_NOSIZE|SWP_NOMOVE

		w.Bind("nextPending", func() *pendingView { return a.nextPendingDL() })
		w.Bind("pendingCount", func() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.pending) })
		w.Bind("dropPending", func(id int) { a.takePending(id) })
		w.Bind("addDownload", func(o addOptions) (int, error) { return a.addWith(o) })
		w.Bind("folderFor", func(name string) string { return a.eng.FolderFor(engine.SafeName(name)) })
		w.Bind("categoryOf", func(name string) string { return engine.CategoryOf(name) })
		w.Bind("fileTypeIcon", func(name string) thumbResult { return a.fileTypeIcon(name) })
		w.Bind("getSettings", func() Settings { return a.getSettings() })
		w.Bind("openMain", func() { go a.show() })
		w.Bind("closeDialog", func() { go closeCapture() })
		w.Bind("pickFolder", func(start string) (string, error) {
			return pickFolder(uintptr(w.Window()), "Choose download folder", start)
		})
		w.SetHtml(i18n.Inject(captureHTML, a.lang()))
		capWin = w
		capMu.Unlock()
		setForeground.Call(hwnd)
		w.Run()
		capMu.Lock()
		capWin = nil
		capMu.Unlock()
		a.clearPending() // closing the dialog cancels what was still waiting
	}()
}

func closeCapture() {
	capMu.Lock()
	w := capWin
	capMu.Unlock()
	if w != nil {
		w.Destroy()
	}
}
