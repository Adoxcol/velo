package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"

	"github.com/Adoxcol/velo/internal/hostreg"
	"github.com/Adoxcol/velo/internal/version"
)

// The setup wizard: Velo's own install and uninstall window (welcome and
// options, progress, finish). It uses WebView2 like the app; without it,
// setup falls back to plain dialogs.

//go:embed wizard.html
var wizardHTML string

type wizardMode string

const (
	modeInstall   wizardMode = "install"
	modeUninstall wizardMode = "uninstall"
)

type wizardInfo struct {
	Mode    wizardMode `json:"mode"`
	Version string     `json:"version"`
	Dir     string     `json:"dir"`
	Upgrade bool       `json:"upgrade"`
	OnPath  bool       `json:"onPath"`
	Desktop bool       `json:"desktop"` // a desktop shortcut exists (or a fresh install)
}

var (
	user32w          = syscall.NewLazyDLL("user32.dll")
	pGetWindowLong   = user32w.NewProc("GetWindowLongPtrW")
	pSetWindowLong   = user32w.NewProc("SetWindowLongPtrW")
	pSetWindowPos    = user32w.NewProc("SetWindowPos")
	pSendMessage     = user32w.NewProc("SendMessageW")
	pSetForeground   = user32w.NewProc("SetForegroundWindow")
	pExtractIcon     = syscall.NewLazyDLL("shell32.dll").NewProc("ExtractIconW")
	pDwmSetAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

// runWizard shows the wizard and returns once it is closed. shown is false
// when WebView2 isn't available, so the caller can use plain dialogs.
func runWizard(mode wizardMode) (shown bool, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dataDir := filepath.Join(os.TempDir(), "velo-setup-webview")
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  dataDir,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: appName + " Setup", Width: 640, Height: 560, Center: true,
		},
	})
	if w == nil {
		return false, nil
	}
	defer os.RemoveAll(dataDir) // best effort; WebView2 may still hold a file
	styleWindow(uintptr(w.Window()))

	dir := installDir()
	var (
		mu      sync.Mutex
		working bool
		done    = make(chan struct{})
		result  error
	)
	progress := func(f float64, s string) {
		w.Dispatch(func() { w.Eval(fmt.Sprintf("progress(%f, %q)", f, s)) })
	}
	start := func(job func() error) bool {
		mu.Lock()
		defer mu.Unlock()
		if working {
			return false
		}
		working = true
		go func() {
			err := job()
			mu.Lock()
			result = err
			mu.Unlock()
			if err != nil {
				w.Dispatch(func() { w.Eval(fmt.Sprintf("failed(%q)", err.Error())) })
			} else {
				w.Dispatch(func() { w.Eval("finished()") })
			}
			close(done)
		}()
		return true
	}

	w.Bind("info", func() wizardInfo {
		up := installed()
		_, err := os.Stat(desktopShortcut())
		return wizardInfo{Mode: mode, Version: version.Version, Dir: dir, Upgrade: up, OnPath: onPath(dir), Desktop: !up || err == nil}
	})
	w.Bind("startInstall", func(o installOptions) bool {
		return start(func() error { return doInstall(dir, o, progress) })
	})
	w.Bind("startUninstall", func(deleteData bool) bool {
		return start(func() error { return doUninstall(dir, deleteData, progress) })
	})
	w.Bind("openStore", func() { openURL(hostreg.ChromeStoreURL) })
	w.Bind("finish", func(launch bool) {
		if launch {
			exec.Command(filepath.Join(dir, "velo.exe")).Start()
		}
		w.Terminate()
	})
	w.Bind("quit", func() { w.Terminate() })
	w.SetHtml(wizardHTML)
	pSetForeground.Call(uintptr(w.Window()))
	w.Run()

	// Closed while working: let the job finish rather than leave Velo half
	// installed or half removed.
	mu.Lock()
	wasWorking := working
	mu.Unlock()
	if wasWorking {
		<-done
	}
	mu.Lock()
	defer mu.Unlock()
	if !wasWorking {
		return true, errCancelled
	}
	if result != nil {
		return true, errCancelled // already shown in the wizard
	}
	return true, nil
}

// styleWindow gives the wizard a fixed size, Velo's icon and, on Windows 11,
// a title bar that follows the system theme.
func styleWindow(hwnd uintptr) {
	const gwlStyle = ^uintptr(15) // -16
	const wsThickFrame, wsMaximizeBox = 0x40000, 0x10000
	style, _, _ := pGetWindowLong.Call(hwnd, gwlStyle)
	pSetWindowLong.Call(hwnd, gwlStyle, style&^(wsThickFrame|wsMaximizeBox))
	pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x1|0x2|0x4|0x20) // NOSIZE|NOMOVE|NOZORDER|FRAMECHANGED

	if self, err := os.Executable(); err == nil {
		p, _ := syscall.UTF16PtrFromString(self)
		if icon, _, _ := pExtractIcon.Call(0, uintptr(unsafe.Pointer(p)), 0); icon > 1 {
			const wmSetIcon = 0x80
			pSendMessage.Call(hwnd, wmSetIcon, 0, icon)
			pSendMessage.Call(hwnd, wmSetIcon, 1, icon)
		}
	}
	if darkMode() {
		on := int32(1)
		const dwmwaUseImmersiveDarkMode = 20
		pDwmSetAttribute.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), 4)
	}
}

// darkMode reports whether Windows apps use the dark theme.
func darkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}
