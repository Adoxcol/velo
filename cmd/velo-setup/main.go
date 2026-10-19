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
	"io/fs"
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

// ---------------------------------------------------------------- install

func install() error {
	if _, err := fs.Stat(payload, "payload/velo.exe"); err != nil {
		return errors.New("this setup was built without Velo's files (run tools/release.ps1)")
	}
	dir := installDir()
	if !silent && !confirm(appName+" Setup", fmt.Sprintf(
		"Install %s %s?\n\nIt will be installed for your user account in:\n%s\n\nNo administrator rights are needed.",
		appName, version.Version, dir)) {
		return errCancelled
	}
	quitRunningVelo()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var size int64
	err := fs.WalkDir(payload, "payload", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "payload/README.md" {
			return err
		}
		rel := strings.TrimPrefix(p, "payload/")
		n, err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), func() (io.ReadCloser, error) { return payload.Open(p) })
		size += n
		return err
	})
	if err != nil {
		return err
	}
	// Keep a copy of this program as the uninstaller.
	self, _ := os.Executable()
	uninstaller := filepath.Join(dir, "velo-setup.exe")
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(uninstaller)) {
		if _, err := writeFile(uninstaller, func() (io.ReadCloser, error) { return os.Open(self) }); err != nil {
			return err
		}
	}

	if err := registerHost(dir); err != nil {
		return fmt.Errorf("registering the browser connection: %w", err)
	}
	registerStoreExtensions()
	if err := createShortcut(startMenuShortcut(), filepath.Join(dir, "velo.exe")); err != nil {
		return fmt.Errorf("creating the Start menu shortcut: %w", err)
	}
	if err := registerUninstall(dir, size); err != nil {
		return fmt.Errorf("registering the uninstaller: %w", err)
	}
	refreshAutostart(dir)

	// Command line: offer to put veloctl on PATH. Silent installs and updates
	// leave the user's earlier choice alone.
	if !silent && !onPath(dir) && confirm(appName+" Setup",
		"Add Velo to PATH?\n\n"+
			"This lets you use the veloctl command-line tool from any terminal, for example:\n"+
			"   veloctl add https://example.com/file.zip\n"+
			"   veloctl list\n\n"+
			"Choose No if you don't use the command line. You can still run veloctl from:\n"+dir) {
		if err := setOnPath(dir, true); err != nil {
			message(appName+" Setup", "Couldn't add Velo to PATH: "+err.Error(), mbIconInfo)
		}
	}

	if silent { // updates and scripted installs: back to the tray, no window
		exec.Command(filepath.Join(dir, "velo.exe"), "-background").Start()
	} else {
		exec.Command(filepath.Join(dir, "velo.exe")).Start()
	}
	if !silent {
		ext := filepath.Join(dir, "extension")
		msg := fmt.Sprintf("%s %s is installed and running in the system tray.\n\n", appName, version.Version)
		if chromeStoreID == "" && edgeStoreID == "" {
			msg += "To connect your browser:\n" +
				"1. Open chrome://extensions (or edge://extensions) and turn on Developer mode.\n" +
				"2. Click \"Load unpacked\" and choose:\n   " + ext + "\n\n" +
				"Open that folder now?"
			if confirm(appName+" Setup", msg) {
				exec.Command("explorer", ext).Start()
			}
		} else {
			msg += "To connect your browser, add the Velo Download Manager extension:\n" +
				"• Chrome: restart Chrome and choose \"Enable extension\" when it says \"New extension added\".\n" +
				"• Edge and Brave: add it from the Chrome Web Store.\n\n" +
				"Open the Chrome Web Store page now?"
			if confirm(appName+" Setup", msg) {
				exec.Command("rundll32", "url.dll,FileProtocolHandler", hostreg.ChromeStoreURL).Start()
			}
		}
	}
	return nil
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

// ---------------------------------------------------------------- uninstall

func uninstall(confirmed bool) error {
	dir := installDir()
	self, _ := os.Executable()
	if !confirmed {
		if !silent && !confirm(appName+" Setup", "Uninstall "+appName+"?\n\nDownloaded files are not touched.") {
			return errCancelled
		}
		// Windows can't delete a running program, so run the uninstall from a
		// temporary copy when started from the install folder.
		if strings.HasPrefix(strings.ToLower(filepath.Clean(self)), strings.ToLower(dir)+`\`) {
			tmp := filepath.Join(os.TempDir(), fmt.Sprintf("velo-uninstall-%d.exe", time.Now().UnixNano()))
			if _, err := writeFile(tmp, func() (io.ReadCloser, error) { return os.Open(self) }); err != nil {
				return err
			}
			args := []string{"/uninstall", "/confirmed"}
			if silent {
				args = append(args, "/S")
			}
			return exec.Command(tmp, args...).Start()
		}
	}

	quitRunningVelo()
	hostreg.Unregister()
	if chromeStoreID != "" {
		registry.DeleteKey(registry.CURRENT_USER, `Software\Google\Chrome\Extensions\`+chromeStoreID)
	}
	if edgeStoreID != "" {
		registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Edge\Extensions\`+edgeStoreID)
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue(appName)
		k.Close()
	}
	registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
	os.Remove(startMenuShortcut())
	if onPath(dir) {
		setOnPath(dir, false)
	}

	var err error
	for i := 0; i < 20; i++ {
		if err = os.RemoveAll(dir); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if !silent && confirm(appName+" Setup", appName+" was removed.\n\nAlso delete your Velo settings and download history?\n(Downloaded files are kept either way.)") {
		os.RemoveAll(ipc.Dir())
	}
	// A temporary uninstaller removes itself once it has exited.
	if strings.HasPrefix(filepath.Base(self), "velo-uninstall-") {
		// Pass the command line verbatim: Go's argument quoting would break cmd's.
		cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine:       `cmd.exe /c ping -n 3 127.0.0.1 >nul & del /f /q "` + self + `"`,
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		cmd.Start()
	}
	if err != nil {
		return fmt.Errorf("some files in %s couldn't be removed: %w", dir, err)
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
