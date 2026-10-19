package main

import (
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

	"golang.org/x/sys/windows/registry"

	"github.com/Adoxcol/velo/internal/hostreg"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

// installOptions are the choices on the setup wizard's first page.
type installOptions struct {
	Path    bool `json:"path"`    // add Velo's folder to PATH (veloctl)
	Desktop bool `json:"desktop"` // desktop shortcut
}

// progressFunc reports install/uninstall progress (0..1) and a status line.
type progressFunc func(frac float64, status string)

func install() error {
	if _, err := fs.Stat(payload, "payload/velo.exe"); err != nil {
		return errors.New("this setup was built without Velo's files (run tools/release.ps1)")
	}
	dir := installDir()
	if silent { // updates and scripted installs: no window, back to the tray
		if err := doInstall(dir, installOptions{}, nil); err != nil {
			return err
		}
		exec.Command(filepath.Join(dir, "velo.exe"), "-background").Start()
		return nil
	}
	if shown, err := runWizard(modeInstall); shown {
		return err
	}
	return installClassic(dir) // no WebView2: plain dialogs
}

// installClassic is the message-box install for PCs without WebView2.
func installClassic(dir string) error {
	if !confirm(appName+" Setup", fmt.Sprintf(
		"Install %s %s?\n\nIt will be installed for your user account in:\n%s\n\nNo administrator rights are needed.",
		appName, version.Version, dir)) {
		return errCancelled
	}
	opts := installOptions{}
	if !onPath(dir) {
		opts.Path = confirm(appName+" Setup", "Add Velo to PATH?\n\nThis lets you use the veloctl command-line tool from any terminal.")
	}
	if err := doInstall(dir, opts, nil); err != nil {
		return err
	}
	exec.Command(filepath.Join(dir, "velo.exe")).Start()
	if confirm(appName+" Setup", appName+" "+version.Version+" is installed and running in the system tray.\n\n"+
		"Add the Velo Download Manager browser extension from the Chrome Web Store now?") {
		openURL(hostreg.ChromeStoreURL)
	}
	return nil
}

// doInstall copies Velo into dir and registers it. Silent updates pass zero
// options, which leaves the user's earlier PATH and shortcut choices alone.
func doInstall(dir string, opts installOptions, progress progressFunc) error {
	report := func(f float64, s string) {
		if progress != nil {
			progress(f, s)
		}
	}
	report(0.02, "Closing Velo…")
	quitRunningVelo()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var files []string
	fs.WalkDir(payload, "payload", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && p != "payload/README.md" {
			files = append(files, p)
		}
		return nil
	})
	var size int64
	for i, p := range files {
		rel := strings.TrimPrefix(p, "payload/")
		report(0.05+0.75*float64(i)/float64(len(files)), "Copying "+filepath.Base(rel)+"…")
		n, err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), func() (io.ReadCloser, error) { return payload.Open(p) })
		if err != nil {
			return err
		}
		size += n
	}
	// Keep a copy of this program as the uninstaller.
	self, _ := os.Executable()
	uninstaller := filepath.Join(dir, "velo-setup.exe")
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(uninstaller)) {
		if _, err := writeFile(uninstaller, func() (io.ReadCloser, error) { return os.Open(self) }); err != nil {
			return err
		}
	}

	report(0.84, "Connecting your browsers…")
	if err := registerHost(dir); err != nil {
		return fmt.Errorf("registering the browser connection: %w", err)
	}
	registerStoreExtensions()
	report(0.9, "Creating shortcuts…")
	if err := createShortcut(startMenuShortcut(), filepath.Join(dir, "velo.exe")); err != nil {
		return fmt.Errorf("creating the Start menu shortcut: %w", err)
	}
	if opts.Desktop {
		createShortcut(desktopShortcut(), filepath.Join(dir, "velo.exe"))
	}
	if err := registerUninstall(dir, size); err != nil {
		return fmt.Errorf("registering the uninstaller: %w", err)
	}
	refreshAutostart(dir)
	if opts.Path && !onPath(dir) {
		report(0.96, "Adding Velo to PATH…")
		if err := setOnPath(dir, true); err != nil {
			return fmt.Errorf("adding Velo to PATH: %w", err)
		}
	}
	report(1, "Done")
	return nil
}

func desktopShortcut() string {
	return filepath.Join(os.Getenv("USERPROFILE"), "Desktop", appName+".lnk")
}

func openURL(u string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
}

// installed reports whether Velo is already installed (an upgrade).
func installed() bool {
	_, err := os.Stat(filepath.Join(installDir(), "velo.exe"))
	return err == nil
}

// ---------------------------------------------------------------- uninstall

func uninstall(relaunched bool) error {
	dir := installDir()
	self, _ := os.Executable()
	// Windows can't delete a running program, so run the uninstall from a
	// temporary copy when started from the install folder.
	if !relaunched && strings.HasPrefix(strings.ToLower(filepath.Clean(self)), strings.ToLower(dir)+`\`) {
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
	defer removeTempUninstaller(self)
	if silent {
		return doUninstall(dir, false, nil)
	}
	if shown, err := runWizard(modeUninstall); shown {
		return err
	}
	if !confirm(appName+" Setup", "Uninstall "+appName+"?\n\nDownloaded files are not touched.") {
		return errCancelled
	}
	err := doUninstall(dir, false, nil)
	if confirm(appName+" Setup", appName+" was removed.\n\nAlso delete your Velo settings and download history?\n(Downloaded files are kept either way.)") {
		os.RemoveAll(ipc.Dir())
	}
	return err
}

// doUninstall removes Velo; deleteData also removes settings and history.
func doUninstall(dir string, deleteData bool, progress progressFunc) error {
	report := func(f float64, s string) {
		if progress != nil {
			progress(f, s)
		}
	}
	report(0.05, "Closing Velo…")
	quitRunningVelo()
	report(0.3, "Disconnecting your browsers…")
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
	report(0.45, "Removing shortcuts…")
	os.Remove(startMenuShortcut())
	os.Remove(desktopShortcut())
	if onPath(dir) {
		setOnPath(dir, false)
	}
	report(0.6, "Removing files…")
	var err error
	for i := 0; i < 20; i++ {
		if err = os.RemoveAll(dir); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if deleteData {
		report(0.9, "Removing settings and history…")
		os.RemoveAll(ipc.Dir())
	}
	report(1, "Done")
	if err != nil {
		return fmt.Errorf("some files in %s couldn't be removed: %w", dir, err)
	}
	return nil
}

// removeTempUninstaller deletes a temporary uninstaller once it has exited.
func removeTempUninstaller(self string) {
	if !strings.HasPrefix(filepath.Base(self), "velo-uninstall-") {
		return
	}
	// Pass the command line verbatim: Go's argument quoting would break cmd's.
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe /c ping -n 3 127.0.0.1 >nul & del /f /q "` + self + `"`,
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	cmd.Start()
}
