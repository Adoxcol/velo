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
	capClean   bool
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
		// No UnlockOSThread: the thread exits with the goroutine, taking any
		// hidden windows (shell, COM, WebView2) with it instead of leaving them
		// on a pooled thread that never reads messages and stalls input.
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

		var closed atomic.Bool
		bindWindowAsync(w, &closed, "nextPending", func(_ any) (*pendingView, error) { return a.nextPendingDL(), nil })
		bindWindowAsync(w, &closed, "pendingCount", func(_ any) (int, error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			return len(a.pending), nil
		})
		bindWindowAsync(w, &closed, "dropPending", func(id int) (bool, error) { a.takePending(id); return true, nil })
		bindWindowAsync(w, &closed, "addDownload", func(o addOptions) (int, error) {
			id, err := a.addWith(o)
			if err == nil && o.Start && a.getSettings().ProgressWindow {
				showProgress(a, id) // IDM-style progress window for this download
			}
			return id, err
		})
		bindWindowAsync(w, &closed, "folderFor", func(name string) (string, error) { return a.eng.FolderFor(engine.SafeName(name)), nil })
		bindWindowAsync(w, &closed, "categoryOf", func(name string) (string, error) { return engine.CategoryOf(name), nil })
		bindWindowAsync(w, &closed, "fileTypeIcon", func(name string) (thumbResult, error) { return a.fileTypeIcon(name), nil })
		bindWindowAsync(w, &closed, "getSettings", func(_ any) (Settings, error) { return a.getSettings(), nil })
		w.Bind("openMain", func() { go a.show() })
		w.Bind("closeDialog", func() { go closeCapture(true) })
		w.Bind("pickFolder", func(start string) (string, error) {
			return pickFolder(uintptr(w.Window()), "Choose download folder", start)
		})
		w.SetHtml(i18n.Inject(captureHTML, a.lang()))
		capWin = w
		capMu.Unlock()
		setForeground.Call(hwnd)
		w.Run()
		closed.Store(true)
		capMu.Lock()
		capWin = nil
		clean := capClean
		capClean = false
		capMu.Unlock()
		if !clean {
			a.clearPending() // closing the dialog via window close cancels what was still waiting
		} else {
			// A download that arrived while the dialog was closing itself
			// would otherwise wait unseen: show it in a new dialog.
			a.mu.Lock()
			left := len(a.pending)
			a.mu.Unlock()
			if left > 0 {
				go showCapture(a)
			}
		}
	}()
}

func closeCapture(clean bool) {
	capMu.Lock()
	w := capWin
	if clean && w != nil {
		capClean = true
	}
	capMu.Unlock()
	if w != nil {
		w.Dispatch(func() {
			w.Destroy()
		})
	}
}
