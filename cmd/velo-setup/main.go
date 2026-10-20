// velo-setup installs and uninstalls Velo for the current user (no admin
// rights needed). The release build embeds velo.exe, velo-host.exe,
// veloctl.exe and the browser extension in payload/.
//
//	velo-setup.exe               install (or upgrade)
//	velo-setup.exe /S            install silently
//	velo-setup.exe /uninstall    uninstall (add /S for silent)
package main

// Icon and manifest (runs without elevation): go generate ./cmd/velo-setup
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico ../velo/assets/velo.ico -manifest setup.manifest -o rsrc_windows_amd64.syso

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"github.com/Adoxcol/velo/internal/hostreg"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

//go:embed all:payload
var payload embed.FS

const appName = "Velo"

// Store extension IDs (set in hostreg once Velo is published). When set,
// the installer also asks the browser to offer the store extension ("New
// extension added") on its next start.
var chromeStoreID, edgeStoreID = hostreg.ChromeStoreID, hostreg.EdgeStoreID

const (
	chromeUpdateURL = "https://clients2.google.com/service/update2/crx"
	edgeUpdateURL   = "https://edge.microsoft.com/extensionwebstorebase/v1/crx"
	uninstallKey    = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Velo`
	runKey          = `Software\Microsoft\Windows\CurrentVersion\Run`
)

var silent bool

func main() {
	args := map[string]bool{}
	for _, a := range os.Args[1:] {
		args[strings.ToLower(strings.TrimLeft(a, "/-"))] = true
	}
	silent = args["s"] || args["silent"]
	var err error
	if args["uninstall"] {
		err = uninstall(args["confirmed"])
	} else {
		err = install()
	}
	if err != nil && !errors.Is(err, errCancelled) {
		message(appName+" Setup", "Something went wrong:\n\n"+err.Error(), mbIconError)
		os.Exit(1)
	}
}

var errCancelled = errors.New("cancelled")

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "Programs", appName)
}

func startMenuShortcut() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`, appName+".lnk")
}

// writeFile replaces dst, retrying while a just-closed Velo still holds it.
func writeFile(dst string, open func() (io.ReadCloser, error)) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	var lastErr error
	for i := 0; i < 40; i++ {
		src, err := open()
		if err != nil {
			return 0, err
		}
		tmp := dst + ".new"
		f, err := os.Create(tmp)
		if err != nil {
			src.Close()
			return 0, err
		}
		n, err := io.Copy(f, src)
		src.Close()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(tmp)
			return 0, err
		}
		if lastErr = os.Rename(tmp, dst); lastErr == nil {
			return n, nil
		}
		os.Remove(tmp)
		time.Sleep(250 * time.Millisecond)
	}
	return 0, fmt.Errorf("%s is in use: %w", filepath.Base(dst), lastErr)
}

// quitRunningVelo asks a running Velo to save and exit. Versions that don't
// understand the request (or don't exit) are stopped; their progress is saved
// every 2 seconds, so little is lost.
func quitRunningVelo() {
	ipc.Call("quit", "", nil)
	for i := 0; i < 40; i++ {
		if !veloRunning() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	cmd := exec.Command("taskkill", "/F", "/IM", "velo.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Run()
	time.Sleep(500 * time.Millisecond)
}

// veloRunning reports whether a velo.exe process exists (any install).
func veloRunning() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq velo.exe", "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, _ := cmd.Output()
	return strings.Contains(strings.ToLower(string(out)), "velo.exe")
}

func registerHost(dir string) error {
	return hostreg.Register(dir)
}

// registerStoreExtensions asks Chrome/Edge to offer the store extension.
func registerStoreExtensions() {
	set := func(path, id, update string) {
		if id == "" {
			return
		}
		if k, _, err := registry.CreateKey(registry.CURRENT_USER, path+id, registry.SET_VALUE); err == nil {
			k.SetStringValue("update_url", update)
			k.Close()
		}
	}
	set(`Software\Google\Chrome\Extensions\`, chromeStoreID, chromeUpdateURL)
	set(`Software\Microsoft\Edge\Extensions\`, edgeStoreID, edgeUpdateURL)
}

func registerUninstall(dir string, size int64) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	setup := filepath.Join(dir, "velo-setup.exe")
	for name, val := range map[string]string{
		"DisplayName":          appName,
		"DisplayVersion":       version.Version,
		"Publisher":            "Velo contributors",
		"DisplayIcon":          filepath.Join(dir, "velo.exe"),
		"InstallLocation":      dir,
		"URLInfoAbout":         "https://github.com/Adoxcol/velo",
		"UninstallString":      `"` + setup + `" /uninstall`,
		"QuietUninstallString": `"` + setup + `" /uninstall /S`,
	} {
		if err := k.SetStringValue(name, val); err != nil {
			return err
		}
	}
	k.SetDWordValue("NoModify", 1)
	k.SetDWordValue("NoRepair", 1)
	k.SetDWordValue("EstimatedSize", uint32(size/1024))
	return nil
}

// refreshAutostart points an existing "start with Windows" entry at the
// installed velo.exe (e.g. after moving from a source build).
func refreshAutostart(dir string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(appName); err == nil {
		k.SetStringValue(appName, `"`+filepath.Join(dir, "velo.exe")+`" -background`)
	}
}

// createShortcut makes a .lnk with PowerShell's WScript.Shell COM object.
func createShortcut(lnk, target string) error {
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.Description='Velo download manager';$s.Save()`,
		esc(lnk), esc(target), esc(filepath.Dir(target)))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------------------------------------------------------------- dialogs

const (
	mbOKCancel  = 0x1
	mbIconError = 0x10
	mbIconQuest = 0x20
	mbIconInfo  = 0x40
	idOK        = 1
)

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func message(title, text string, flags uintptr) uintptr {
	if silent {
		return idOK
	}
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return r
}

func confirm(title, text string) bool { return message(title, text, mbOKCancel|mbIconQuest) == idOK }
