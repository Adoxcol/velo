//go:build windows

package main

import (
	"log"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Windows notifications via Shell_NotifyIcon balloons, which Windows 10/11
// show as toast notifications. A hidden message-only window owns a second,
// short-lived tray icon that carries the notification and reports clicks.
// (The main tray icon belongs to the systray library, which can't do this.)

var (
	user32n              = syscall.NewLazyDLL("user32.dll")
	shell32n             = syscall.NewLazyDLL("shell32.dll")
	kernel32n            = syscall.NewLazyDLL("kernel32.dll")
	pRegisterClassExW    = user32n.NewProc("RegisterClassExW")
	pCreateWindowExW     = user32n.NewProc("CreateWindowExW")
	pDefWindowProcW      = user32n.NewProc("DefWindowProcW")
	pGetMessageW         = user32n.NewProc("GetMessageW")
	pTranslateMessage    = user32n.NewProc("TranslateMessage")
	pDispatchMessageW    = user32n.NewProc("DispatchMessageW")
	pCreateIconFromResEx = user32n.NewProc("CreateIconFromResourceEx")
	pShellNotifyIconW    = shell32n.NewProc("Shell_NotifyIconW")
	pGetModuleHandleW    = kernel32n.NewProc("GetModuleHandleW")
)

const (
	nimAdd, nimModify, nimDelete = 0, 1, 2
	nifMessage, nifIcon, nifTip  = 0x1, 0x2, 0x4
	nifInfo                      = 0x10
	niifInfo, niifUser           = 0x1, 0x4
	niifLargeIcon                = 0x20
	ninBalloonUserClick          = 0x405
	wmApp                        = 0x8000
	wmNotify                     = wmApp + 1
	hwndMessage                  = ^uintptr(2) // HWND_MESSAGE (-3)
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         syscall.GUID
	HBalloonIcon     uintptr
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	_       uint32
}

type notifier struct {
	mu      sync.Mutex
	hwnd    uintptr
	icon    uintptr
	shown   bool
	gen     int    // bumps with each notification, so old timers don't remove new ones
	onClick func() // action for the current notification
}

// toast is set up by runTray; nil (no notifications) in tests and the CLI.
var toast *notifier

// startNotifier creates the hidden window on its own thread.
func startNotifier() *notifier {
	n := &notifier{}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		hinst, _, _ := pGetModuleHandleW.Call(0)
		cls, _ := syscall.UTF16PtrFromString("VeloNotify")
		wc := wndClassEx{LpfnWndProc: syscall.NewCallback(n.wndProc), HInstance: hinst, LpszClassName: cls}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, hinst, 0)
		if hwnd == 0 {
			log.Printf("notifications unavailable: %v", err)
			close(ready)
			return
		}
		n.hwnd = hwnd
		n.icon = makeHIcon()
		close(ready)
		var m winMsg
		for {
			if r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	<-ready
	return n
}

func (n *notifier) wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmNotify && lp&0xFFFF == ninBalloonUserClick {
		n.mu.Lock()
		f := n.onClick
		n.onClick = nil
		n.mu.Unlock()
		if f != nil {
			go f()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// makeHIcon is the 32 px app icon used in notifications.
func makeHIcon() uintptr { return hIcon(32) }

func copyUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)-1]
		u = append(u, 0)
	}
	copy(dst, u)
}

// Show displays a notification; onClick (may be nil) runs if it's clicked.
func (n *notifier) Show(title, body string, onClick func()) {
	if n == nil || n.hwnd == 0 {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	nid := notifyIconData{HWnd: n.hwnd, UID: 1, UCallbackMessage: wmNotify, HIcon: n.icon}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	copyUTF16(nid.SzTip[:], "Velo")
	if !n.shown {
		nid.UFlags = nifMessage | nifIcon | nifTip
		if r, _, err := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
			log.Printf("notification icon: %v", err)
			return
		}
		n.shown = true
	}
	nid.UFlags = nifInfo | nifIcon | nifTip | nifMessage
	nid.DwInfoFlags = niifUser | niifLargeIcon // our icon is 32x32
	nid.HBalloonIcon = n.icon
	copyUTF16(nid.SzInfoTitle[:], title)
	copyUTF16(nid.SzInfo[:], body)
	if r, _, err := pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid))); r == 0 {
		// Fall back to the standard info icon if a custom balloon icon is refused.
		nid.DwInfoFlags, nid.HBalloonIcon = niifInfo, 0
		if r, _, err2 := pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid))); r == 0 {
			log.Printf("notification failed: %v / %v", err, err2)
		} else {
			log.Printf("notification shown with the standard icon (custom icon: %v)", err)
		}
	}
	n.onClick = onClick
	n.gen++
	gen := n.gen
	// Remove the helper icon once the notification has had time to be seen
	// (clicks from the Action Center after that are ignored).
	time.AfterFunc(20*time.Second, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.gen == gen {
			n.remove()
		}
	})
}

// remove deletes the helper tray icon; n.mu must be held.
func (n *notifier) remove() {
	if !n.shown {
		return
	}
	nid := notifyIconData{HWnd: n.hwnd, UID: 1}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	n.shown = false
	n.onClick = nil
}

// Close removes any notification icon (call on exit).
func (n *notifier) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.remove()
	n.mu.Unlock()
}
