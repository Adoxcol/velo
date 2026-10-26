//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
	webview2 "github.com/jchv/go-webview2"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/hostreg"
	"github.com/Adoxcol/velo/internal/i18n"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

//go:embed ui.html
var uiHTML string

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	showWindow    = user32.NewProc("ShowWindow")
	setForeground = user32.NewProc("SetForegroundWindow")
	winMu         sync.Mutex
	opening       bool
	win           webview2.WebView // nil when the window is closed
)

// runTray shows the tray icon and blocks until the user picks Exit.
func runTray(a *app, openWindow bool) {
	setupTrayApp(a)
	systray.Run(func() {
		setupTrayUI(a, openWindow)
	}, nil)
}

// setupTrayApp configures application callbacks and starts background workers.
func setupTrayApp(a *app) {
	a.show = func() { showMain(a) }
	var quitOnce sync.Once
	a.quit = func() {
		quitOnce.Do(func() {
			a.shutdown()
			closeMain()
			closeMini()
			toast.Close()
			systray.Quit()
		})
	}
	toast = startNotifier()
	a.onDone = func(d *engine.Download, autoOpened bool) {
		p, _ := d.Info()
		systray.SetTooltip("Velo — finished " + filepath.Base(p))
		if a.getSettings().CompleteDialog && !autoOpened { // already opening by itself otherwise
			showComplete(a, d.ID) // IDM-style "Download complete" window
			return
		}
		if a.getSettings().Notify {
			toast.Show(a.tr("Download complete"), filepath.Base(p)+"\n"+a.tr("Click to show it in its folder."), func() {
				a.openFile(d.ID, true)
			})
		}
	}
	a.onFail = func(d *engine.Download, err error) {
		if a.getSettings().Notify {
			name := d.Req.Filename
			if p, _ := d.Info(); p != "" {
				name = filepath.Base(p)
			}
			if name == "" {
				name = linkName(d.Req.URL)
			}
			toast.Show(a.tr("Download failed"), name+"\n"+err.Error(), a.show)
		}
	}
	go a.watchClipboard()
	go a.runMini()
	go a.upd.run(a)
	a.showPending = func() { showCapture(a) } // its own small window, like IDM
	a.showBatch = func() {
		a.show()
		evalMain("openBatch()")
	}
}

type trayMenu struct {
	mOpen      *systray.MenuItem
	mAdd       *systray.MenuItem
	mFolder    *systray.MenuItem
	mPauseAll  *systray.MenuItem
	mResumeAll *systray.MenuItem
	mMini      *systray.MenuItem
	mSched     *systray.MenuItem
	mQuit      *systray.MenuItem
}

// setupTrayUI initializes the tray icon, builds its menu, and attaches event listeners.
func setupTrayUI(a *app, openWindow bool) {
	systray.SetIcon(appIcon())
	systray.SetTitle("Velo")
	systray.SetTooltip("Velo")
	systray.SetOnTapped(a.show) // left click

	menu := buildTrayMenu(a)
	handleTrayEvents(a, menu)

	if openWindow {
		a.show()
	}
}

// buildTrayMenu constructs the tray items, language titles, and speed limit controls.
func buildTrayMenu(a *app) trayMenu {
	// Menu items remember their English text so a language change can retitle them.
	type titled struct {
		m       *systray.MenuItem
		en, tip string
	}
	var titledItems []titled
	item := func(m *systray.MenuItem, en, tip string) *systray.MenuItem {
		titledItems = append(titledItems, titled{m, en, tip})
		m.SetTitle(a.tr(en))
		m.SetTooltip(a.tr(tip))
		return m
	}

	mOpen := item(systray.AddMenuItem("Show Velo", ""), "Show Velo", "Show the download list")
	mAdd := item(systray.AddMenuItem("Add Download…", ""), "Add Download…", "Add a link")
	mFolder := item(systray.AddMenuItem("Open downloads folder", ""), "Open downloads folder", "")
	systray.AddSeparator()
	mPauseAll := item(systray.AddMenuItem("Pause all", ""), "Pause all", "")
	mResumeAll := item(systray.AddMenuItem("Resume all", ""), "Resume all", "")
	systray.AddSeparator()
	mLimit := item(systray.AddMenuItem("Speed limit", ""), "Speed limit", "")

	limits := []int{0, 256, 1024, 5 * 1024} // KB/s
	var limitItems []*systray.MenuItem
	for _, kb := range limits {
		label := "Unlimited"
		if kb > 0 {
			label = sizeKB(kb) + "/s"
		}
		limitItems = append(limitItems, item(mLimit.AddSubMenuItemCheckbox(label, "", false), label, ""))
	}
	mMini := item(systray.AddMenuItemCheckbox("Show Velo Mini", "", a.getSettings().ShowMini), "Show Velo Mini", "The small progress window while downloading")

	syncLimit := func() {
		if a.getSettings().ShowMini {
			mMini.Check()
		} else {
			mMini.Uncheck()
		}
		cur := a.getSettings().SpeedLimitKB
		for i, kb := range limits {
			if kb == cur {
				limitItems[i].Check()
			} else {
				limitItems[i].Uncheck()
			}
		}
	}
	syncLimit()

	lang := a.lang()
	a.changed = func() {
		syncLimit()
		if l := a.lang(); l != lang { // language changed: retitle the menu, reload the windows
			lang = l
			for _, t := range titledItems {
				t.m.SetTitle(a.tr(t.en))
				t.m.SetTooltip(a.tr(t.tip))
			}
			go reloadWindows(a)
		}
	}
	a.onTick = a.updateTaskbar

	for i, kb := range limits {
		ch := limitItems[i].ClickedCh
		speedKB := kb
		go func() {
			for range ch {
				a.setSpeedLimit(speedKB)
			}
		}()
	}

	mSched := item(systray.AddMenuItem("Settings…", ""), "Settings…", "Download folder, scheduler, when finished")
	systray.AddSeparator()
	mQuit := item(systray.AddMenuItem("Exit", ""), "Exit", "Pause everything and quit")

	return trayMenu{
		mOpen:      mOpen,
		mAdd:       mAdd,
		mFolder:    mFolder,
		mPauseAll:  mPauseAll,
		mResumeAll: mResumeAll,
		mMini:      mMini,
		mSched:     mSched,
		mQuit:      mQuit,
	}
}

// handleTrayEvents processes user clicks on tray menu items in a background goroutine.
func handleTrayEvents(a *app, m trayMenu) {
	go func() {
		for {
			select {
			case <-m.mAdd.ClickedCh:
				a.show()
				evalMain("openAdd()")
			case <-m.mOpen.ClickedCh:
				a.show()
			case <-m.mFolder.ClickedCh:
				a.openDir()
			case <-m.mPauseAll.ClickedCh:
				a.all("pause")
			case <-m.mResumeAll.ClickedCh:
				a.all("resume")
			case <-m.mMini.ClickedCh:
				a.updateSettings(func(s *Settings) { s.ShowMini = !s.ShowMini })
			case <-m.mSched.ClickedCh:
				a.show()
				evalMain("go('settings')")
			case <-m.mQuit.ClickedCh:
				a.quit()
				return
			}
		}
	}()
}

// all pauses or resumes every download it applies to.
func (a *app) all(op string) {
	for _, r := range a.rows() {
		if op == "pause" {
			a.pause(r.ID)
		} else {
			a.resume(r.ID)
		}
	}
}

// shutdown stops running downloads so their progress is saved.
func (a *app) shutdown() {
	a.mu.Lock()
	for _, it := range a.items {
		if it.running {
			it.cancel()
		}
	}
	a.mu.Unlock()
	for i := 0; i < 60; i++ {
		a.mu.Lock()
		n := a.running
		a.mu.Unlock()
		if n == 0 {
			break
		}
		sleepMs(50)
	}
	a.saveHistory()
}

// showMain opens the download window, or brings it to the front.
func showMain(a *app) {
	winMu.Lock()
	defer winMu.Unlock()
	if win != nil {
		hwnd := uintptr(win.Window())
		showWindow.Call(hwnd, 9) // SW_RESTORE
		setForeground.Call(hwnd)
		return
	}
	if opening {
		return
	}
	opening = true
	go func() {
		runtime.LockOSThread() // a window and its message loop share a thread
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath:  filepath.Join(ipc.Dir(), "webview"),
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title: "Velo", Width: 1180, Height: 760, Center: true,
			},
		})
		winMu.Lock()
		opening = false
		if w == nil {
			winMu.Unlock()
			log.Print("could not create window (is the WebView2 runtime installed?)")
			return
		}
		setWindowIcon(uintptr(w.Window()))
		var closed atomic.Bool
		bind(w, a, &closed)
		w.SetHtml(i18n.Inject(uiHTML, a.lang()))
		win = w
		winMu.Unlock()
		w.Run() // until the window is closed
		closed.Store(true)
		winMu.Lock()
		win = nil
		winMu.Unlock()
	}()
}

// evalMain runs JavaScript in the window once it is open.
func evalMain(js string) {
	for i := 0; i < 40; i++ {
		winMu.Lock()
		w := win
		winMu.Unlock()
		if w != nil {
			sleepMs(300) // let the page load
			w.Dispatch(func() { w.Eval(js) })
			return
		}
		sleepMs(100)
	}
}

func closeMain() {
	winMu.Lock()
	w := win
	winMu.Unlock()
	if w != nil {
		w.Destroy()
	}
}

// bind exposes the app to the page's JavaScript (each returns a Promise).
// bind exposes the app to the main window. Calls that can take seconds
// (thumbnails via ffmpeg or the shell, opening files and Explorer) run off
// the window thread with bindWindowAsync, so the window never freezes.
func bind(w webview2.WebView, a *app, closed *atomic.Bool) {
	w.Bind("probe", func(url, user, pass string) (engine.Info, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return a.eng.Probe(ctx, engine.Request{URL: strings.TrimSpace(url), User: user, Pass: pass})
	})
	w.Bind("addDownload", func(o addOptions) (int, error) { return a.addWith(o) })
	w.Bind("takeBatch", func() *batchView { return a.takeBatch() })
	w.Bind("nextPending", func() *pendingView { return a.nextPendingDL() })
	w.Bind("dropPending", func(id int) { a.takePending(id) })
	w.Bind("finishBatch", func(urls []string, start bool) int { return a.finishBatch(urls, start) })
	w.Bind("extensionSeen", func() int64 { return a.extensionSeen() })
	w.Bind("appVersion", func() string { return version.Version })
	w.Bind("pendingReleaseNotes", a.pendingReleaseNotes)
	w.Bind("dismissReleaseNotes", dismissReleaseNotes)
	w.Bind("updateInfo", func() updateInfo { return a.upd.info() })
	w.Bind("debugInfo", func() string { return a.debugInfo() })
	w.Bind("languages", func() map[string]any {
		return map[string]any{"list": i18n.Languages, "current": a.lang()}
	})
	w.Bind("setQueue", func(id int, q string) error { return a.setQueue(id, q) })
	w.Bind("reorder", func(id int, how string) error { return a.reorder(id, how) })
	w.Bind("refreshLink", func(id int, u string) error { return a.refreshLink(id, u) })
	w.Bind("checkUpdate", func() { go a.upd.check(a) })
	w.Bind("installUpdate", func() error { return a.upd.install(a) })
	w.Bind("copyText", func(t string) error { return setClipboardText(t) })
	w.Bind("pasteText", func() string {
		t, _ := clipboardText()
		return strings.TrimSpace(t)
	})
	w.Bind("connections", func() int { return a.eng.SegmentsDefault() })
	w.Bind("appStatus", func() status { return a.status() })
	w.Bind("getSettings", func() Settings { return a.getSettings() })
	w.Bind("setSettings", func(s Settings) error { return a.setSettings(s) })
	w.Bind("setSpeedLimit", func(kb int) error { return a.setSpeedLimit(kb) })
	w.Bind("add", func(rawURL string) (int, error) {
		u := strings.ToLower(rawURL)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return 0, errors.New("enter a link starting with http:// or https://")
		}
		_, kind := engine.Downloadable(rawURL)
		id := a.add(engine.Request{URL: rawURL, Kind: kind})
		if id == 0 {
			return 0, errors.New("invalid URL")
		}
		return id, nil
	})
	w.Bind("pause", func(id int) error { return a.pause(id) })
	w.Bind("resume", func(id int) error { return a.resume(id) })
	w.Bind("remove", func(id int) error { return a.remove(id) })
	w.Bind("deleteFile", func(id int) error { return a.deleteFile(id) })
	w.Bind("toggleOpenOnDone", func(id int) (bool, error) { return a.toggleOpenOnDone(id) })
	w.Bind("expandPlaylist", func(url string) (int, error) { return a.expandPlaylistToBatch(url) })
	w.Bind("exportLinks", func(ids []int) (string, error) {
		txt := a.exportLinks(ids)
		if txt == "" {
			return "", nil
		}
		path, err := pickSaveFile(uintptr(w.Window()), "Export download links", "velo-links.txt")
		if err != nil || path == "" {
			return "", err
		}
		if err := os.WriteFile(path, []byte(txt), 0o644); err != nil {
			return "", err
		}
		return path, nil
	})
	w.Bind("loadBatchLinks", func(urls []string, source string) int { return a.loadBatchLinks(urls, source) })
	w.Bind("videoToolsStatus", func() videoToolsState { return a.videoToolsStatus() })
	w.Bind("installVideoTools", func() error { return a.installVideoTools() })
	bindWindowAsync(w, closed, "openFile", func(id int) (bool, error) { return true, a.openFile(id, false) })
	bindWindowAsync(w, closed, "showInFolder", func(id int) (bool, error) { return true, a.openFile(id, true) })
	bindWindowAsync(w, closed, "openDir", func(_ any) (bool, error) { return true, a.openDir() })
	w.Bind("openStore", func() error {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", hostreg.ChromeStoreURL).Start()
	})
	w.Bind("openFirefoxStore", func() error {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", hostreg.FirefoxStoreURL).Start()
	})
	w.Bind("pauseAll", func() { a.all("pause") })
	w.Bind("clearDone", func() int { return a.clearDone() })
	w.Bind("resumeAll", func() { a.all("resume") })
	w.Bind("downloadDir", func() string { return a.eng.Dir() })
	w.Bind("defaultDir", func() string { return a.defaultDir })
	bindWindowAsync(w, closed, "itemPreview", func(id int) (thumbResult, error) { return a.itemPreview(id), nil })
	bindWindowAsync(w, closed, "fileTypeIcon", func(name string) (thumbResult, error) { return a.fileTypeIcon(name), nil })
	// Runs on the window's thread, so the picker is modal to it.
	w.Bind("pickFolder", func(start string) (string, error) {
		return pickFolder(uintptr(w.Window()), "Choose download folder", start)
	})
}

// reloadWindows redraws open windows in the current language. Velo Mini and
// the download dialog are closed; they reopen in the new language when needed.
func reloadWindows(a *app) {
	closeMini()
	winMu.Lock()
	w := win
	winMu.Unlock()
	if w != nil {
		w.Dispatch(func() {
			w.SetHtml(i18n.Inject(uiHTML, a.lang()))
		})
	}
}
