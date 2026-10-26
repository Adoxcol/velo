//go:build windows

package main

import (
	_ "embed"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/i18n"
	"github.com/Adoxcol/velo/internal/ipc"
)

// The "Download complete" window, like IDM's: when a download finishes it
// shows the file with Open, Open with…, Open folder and Close. One window at
// a time; a download that finishes while it is open replaces its contents.

//go:embed complete.html
var completeHTML string

var (
	doneMu      sync.Mutex
	doneWin     webview2.WebView
	doneOpening bool
	doneID      int // the download the window shows
)

const doneW, doneH = 560, 252

type completeView struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Size     int64  `json:"size"`
	Category string `json:"category"`
	Exists   bool   `json:"exists"`
}

func (a *app) completeInfo() *completeView {
	doneMu.Lock()
	id := doneID
	doneMu.Unlock()
	it := a.get(id)
	if it == nil {
		return nil
	}
	p, total := it.d.Info()
	v := &completeView{ID: id, Name: filepath.Base(p), Dir: filepath.Dir(p), Size: total, Category: engine.CategoryOf(filepath.Base(p))}
	if fi, err := os.Stat(p); err == nil {
		v.Exists = true
		v.Size = fi.Size()
	}
	return v
}

// openWith shows Windows' "Open with" chooser for a finished download.
func (a *app) openWith(id int) error {
	it := a.get(id)
	if it == nil {
		return errors.New("no such download")
	}
	p, _ := it.d.Info()
	return exec.Command("rundll32", "shell32.dll,OpenAs_RunDLL", p).Start()
}

// showComplete opens the window for download id, or shows id in the window
// that is already open.
func showComplete(a *app, id int) {
	doneMu.Lock()
	defer doneMu.Unlock()
	doneID = id
	if doneWin != nil {
		w := doneWin
		w.Dispatch(func() {
			w.Eval("window.load && load()")
			showWindow.Call(uintptr(w.Window()), 4) // SW_SHOWNOACTIVATE
		})
		return
	}
	if doneOpening {
		return
	}
	doneOpening = true
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath: filepath.Join(ipc.Dir(), "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: "Velo — Download complete", Width: doneW, Height: doneH, Center: true,
			},
		})
		doneMu.Lock()
		doneOpening = false
		if w == nil {
			doneMu.Unlock()
			log.Print("could not create the download complete window")
			return
		}
		hwnd := uintptr(w.Window())
		setWindowIcon(hwnd)
		const wsThickFrame, wsMaximizeBox = 0x40000, 0x10000
		style, _, _ := pGetWindowLongPtr.Call(hwnd, index(gwlStyle))
		pSetWindowLongPtr.Call(hwnd, index(gwlStyle), style&^(wsThickFrame|wsMaximizeBox))
		pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x1|0x2|0x4|swpFrameChanged)

		var closed atomic.Bool
		bindWindowAsync(w, &closed, "completeInfo", func(_ int) (*completeView, error) { return a.completeInfo(), nil })
		bindWindowAsync(w, &closed, "itemPreview", func(id int) (thumbResult, error) { return a.itemPreview(id), nil })
		bindWindowAsync(w, &closed, "fileTypeIcon", func(name string) (thumbResult, error) { return a.fileTypeIcon(name), nil })
		w.Bind("getSettings", func() Settings { return a.getSettings() })
		bindWindowAsync(w, &closed, "openDone", func(id int) (bool, error) { return true, a.openFile(id, false) })
		bindWindowAsync(w, &closed, "openDoneWith", func(id int) (bool, error) { return true, a.openWith(id) })
		bindWindowAsync(w, &closed, "showDoneInFolder", func(id int) (bool, error) { return true, a.openFile(id, true) })
		bindWindowAsync(w, &closed, "closeComplete", func(dontShowAgain bool) (bool, error) {
			if dontShowAgain {
				if err := a.updateSettings(func(s *Settings) { s.CompleteDialog = false }); err != nil {
					return false, err
				}
			}
			// Target this window, not whichever completion window is current
			// when a slow settings save finishes.
			w.Dispatch(func() {
				if !closed.Load() {
					w.Destroy()
				}
			})
			return true, nil
		})
		w.SetHtml(i18n.Inject(completeHTML, a.lang()))
		doneWin = w
		doneMu.Unlock()
		w.Run()
		closed.Store(true)
		doneMu.Lock()
		doneWin = nil
		doneMu.Unlock()
	}()
}

func closeComplete() {
	doneMu.Lock()
	w := doneWin
	doneMu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Destroy() })
	}
}
