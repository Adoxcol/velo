//go:build windows

package main

import (
	_ "embed"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"

	"velo/internal/i18n"
	"velo/internal/ipc"
)

// Velo Mini: a small always-on-top window in the bottom-right corner that
// shows download progress while the main window is closed.

//go:embed mini.html
var miniHTML string

var (
	pSetWindowLongPtr = user32n.NewProc("SetWindowLongPtrW")
	pGetWindowLongPtr = user32n.NewProc("GetWindowLongPtrW")
	pSetWindowPos     = user32n.NewProc("SetWindowPos")
	pIsWindowVisible  = user32n.NewProc("IsWindowVisible")
	pIsIconic         = user32n.NewProc("IsIconic")
	pSysParamsInfo    = user32n.NewProc("SystemParametersInfoW")
	pDwmSetAttr       = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	pReleaseCapture   = user32n.NewProc("ReleaseCapture")
	pPostMessage      = user32n.NewProc("PostMessageW")
	pGetWindowRect    = user32n.NewProc("GetWindowRect")
	pGetSystemMetrics = user32n.NewProc("GetSystemMetrics")

	miniMu      sync.Mutex
	mini        webview2.WebView
	miniOpening bool
)

const (
	gwlStyle, gwlExStyle                             = -16, -20
	wsPopup                                          = 0x80000000
	wsVisible                                        = 0x10000000
	wsExToolWindow                                   = 0x80
	wsExTopmost                                      = 0x8
	swpNoActivate                                    = 0x10
	swpFrameChanged                                  = 0x20
	swpShowWindow                                    = 0x40
	hwndTopmost                                      = ^uintptr(0) // -1
	spiGetWorkArea                                   = 0x30
	miniW, miniH                                     = 440, 104
	wmNCLButtonDown                                  = 0xA1
	htCaption                                        = 2
	smXVirtual, smYVirtual, smCXVirtual, smCYVirtual = 76, 77, 78, 79
)

// miniPlace is where Velo Mini opens: where it was last dragged to if that
// is still on a screen, else the bottom-right corner.
func miniPlace(saved *[2]int) (int32, int32) {
	if saved != nil {
		m := func(i uintptr) int32 { v, _, _ := pGetSystemMetrics.Call(i); return int32(v) }
		x, y := int32(saved[0]), int32(saved[1])
		vx, vy := m(smXVirtual), m(smYVirtual)
		if x >= vx && y >= vy && x+miniW <= vx+m(smCXVirtual) && y+miniH <= vy+m(smCYVirtual) {
			return x, y
		}
	}
	var wa rect
	pSysParamsInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	return wa.R - miniW - 16, wa.B - miniH - 16
}

type rect struct{ L, T, R, B int32 }

// index converts a negative GWL_* index to the uintptr the API expects.
func index(i int) uintptr { return uintptr(i) }

// showMini opens Velo Mini if it isn't open.
func showMini(a *app) {
	miniMu.Lock()
	defer miniMu.Unlock()
	if mini != nil || miniOpening {
		return
	}
	miniOpening = true
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath: filepath.Join(ipc.Dir(), "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: "Velo Mini", Width: miniW, Height: miniH,
			},
		})
		miniMu.Lock()
		miniOpening = false
		if w == nil {
			miniMu.Unlock()
			log.Print("could not create Velo Mini")
			return
		}
		hwnd := uintptr(w.Window())
		setWindowIcon(hwnd)
		// Borderless, no taskbar button, always on top, rounded corners.
		pSetWindowLongPtr.Call(hwnd, index(gwlStyle), wsPopup|wsVisible)
		ex, _, _ := pGetWindowLongPtr.Call(hwnd, index(gwlExStyle))
		pSetWindowLongPtr.Call(hwnd, index(gwlExStyle), ex|wsExToolWindow|wsExTopmost)
		corner := uint32(2) // DWMWCP_ROUND
		pDwmSetAttr.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), 4)
		x, y := miniPlace(a.getSettings().MiniPos)
		pSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), miniW, miniH, swpNoActivate|swpFrameChanged|swpShowWindow)

		w.Bind("appStatus", func() status { return a.status() })
		w.Bind("getSettings", func() Settings { return a.getSettings() })
		w.Bind("pause", func(id int) error { return a.pause(id) })
		w.Bind("resume", func(id int) error { return a.resume(id) })
		w.Bind("openMain", func() { go a.show() })
		// mode "now": until these downloads finish; "never": turn Velo Mini off.
		w.Bind("hideMini", func(mode string) {
			if mode == "never" {
				a.updateSettings(func(s *Settings) { s.ShowMini = false })
			} else {
				a.mu.Lock()
				a.miniDismissed = true
				a.mu.Unlock()
			}
			go closeMini()
		})
		w.Bind("dragMini", func() { // the window has no title bar: drag it from anywhere
			pReleaseCapture.Call()
			pPostMessage.Call(hwnd, wmNCLButtonDown, htCaption, 0)
		})
		w.Bind("miniMoved", func() { // after a drag: remember where it is
			var r rect
			pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
			pos := [2]int{int(r.L), int(r.T)}
			if cur := a.getSettings().MiniPos; cur == nil || *cur != pos {
				a.updateSettings(func(s *Settings) { s.MiniPos = &pos })
			}
		})
		w.SetHtml(i18n.Inject(miniHTML, a.lang()))
		mini = w
		miniMu.Unlock()
		w.Run()
		miniMu.Lock()
		mini = nil
		miniMu.Unlock()
	}()
}

func hasMiniWindow() bool {
	miniMu.Lock()
	defer miniMu.Unlock()
	return mini != nil
}

func closeMini() {
	miniMu.Lock()
	w := mini
	miniMu.Unlock()
	if w != nil {
		w.Destroy()
	}
}

// mainVisible reports whether the main window is open and not minimised.
func mainVisible() bool {
	winMu.Lock()
	defer winMu.Unlock()
	if win == nil {
		return false
	}
	h := uintptr(win.Window())
	v, _, _ := pIsWindowVisible.Call(h)
	ic, _, _ := pIsIconic.Call(h)
	return v != 0 && ic == 0
}

// runMini shows Velo Mini while downloads are running and the main window
// isn't, and hides it a few seconds after they stop.
func (a *app) runMini() {
	idleSince := time.Time{}
	for range time.Tick(time.Second) {
		a.mu.Lock()
		active := a.running > 0
		if !active {
			a.miniDismissed = false // next downloads may show it again
		}
		dismissed := a.miniDismissed
		a.mu.Unlock()
		enabled := a.getSettings().ShowMini
		if active {
			idleSince = time.Time{}
		} else if idleSince.IsZero() {
			idleSince = time.Now()
		}
		switch {
		case !enabled || dismissed || mainVisible():
			closeMini()
		case active:
			showMini(a)
		case time.Since(idleSince) > 4*time.Second:
			closeMini()
		}
	}
}
