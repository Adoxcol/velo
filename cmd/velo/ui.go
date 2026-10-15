package main

import (
	"context"
	_ "embed"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
	webview2 "github.com/jchv/go-webview2"

	"velo/internal/engine"
	"velo/internal/ipc"
	"velo/internal/version"
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
	a.onDone = func(d *engine.Download) {
		p, _ := d.Info()
		systray.SetTooltip("Velo — finished " + filepath.Base(p))
		if a.getSettings().Notify {
			toast.Show("Download complete", filepath.Base(p)+"\nClick to show it in its folder.", func() {
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
			toast.Show("Download failed", name+"\n"+err.Error(), a.show)
		}
	}
	go a.watchClipboard()
	go a.runMini()
	go a.upd.run(a)
	a.showPending = func() {
		a.show()
		evalMain("openPending()")
	}
	a.showBatch = func() {
		a.show()
		evalMain("openBatch()")
	}
	systray.Run(func() {
		systray.SetIcon(appIcon())
		systray.SetTitle("Velo")
		systray.SetTooltip("Velo")
		systray.SetOnTapped(a.show) // left click
		mOpen := systray.AddMenuItem("Show Velo", "Show the download list")
		mAdd := systray.AddMenuItem("Add Download…", "Add a link")
		mFolder := systray.AddMenuItem("Open downloads folder", "")
		systray.AddSeparator()
		mPauseAll := systray.AddMenuItem("Pause all", "")
		mResumeAll := systray.AddMenuItem("Resume all", "")
		systray.AddSeparator()
		mLimit := systray.AddMenuItem("Speed limit", "")
		limits := []int{0, 256, 1024, 5 * 1024} // KB/s
		var limitItems []*systray.MenuItem
		for _, kb := range limits {
			label := "Unlimited"
			if kb > 0 {
				label = sizeKB(kb) + "/s"
			}
			limitItems = append(limitItems, mLimit.AddSubMenuItemCheckbox(label, "", false))
		}
		mMini := systray.AddMenuItemCheckbox("Show Velo Mini", "The small progress window while downloading", a.getSettings().ShowMini)
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
		a.changed = syncLimit
		for i, kb := range limits {
			go func() {
				for range limitItems[i].ClickedCh {
					a.setSpeedLimit(kb)
				}
			}()
		}
		mSched := systray.AddMenuItem("Settings…", "Download folder, scheduler, when finished")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Exit", "Pause everything and quit")
		go func() {
			for {
				select {
				case <-mAdd.ClickedCh:
					a.show()
					evalMain("openAdd()")
				case <-mOpen.ClickedCh:
					a.show()
				case <-mFolder.ClickedCh:
					a.openDir()
				case <-mPauseAll.ClickedCh:
					a.all("pause")
				case <-mResumeAll.ClickedCh:
					a.all("resume")
				case <-mMini.ClickedCh:
					a.updateSettings(func(s *Settings) { s.ShowMini = !s.ShowMini })
				case <-mSched.ClickedCh:
					a.show()
					evalMain("go('settings')")
				case <-mQuit.ClickedCh:
					a.quit()
					return
				}
			}
		}()
		if openWindow {
			a.show()
		}
	}, nil)
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
		bind(w, a)
		w.SetHtml(uiHTML)
		win = w
		winMu.Unlock()
		w.Run() // until the window is closed
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
func bind(w webview2.WebView, a *app) {
	w.Bind("probe", func(url string) (engine.Info, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return a.eng.Probe(ctx, engine.Request{URL: strings.TrimSpace(url)})
	})
	w.Bind("addDownload", func(o addOptions) (int, error) { return a.addWith(o) })
	w.Bind("takeBatch", func() *batchView { return a.takeBatch() })
	w.Bind("nextPending", func() *pendingView { return a.nextPendingDL() })
	w.Bind("dropPending", func(id int) { a.takePending(id) })
	w.Bind("finishBatch", func(urls []string, start bool) int { return a.finishBatch(urls, start) })
	w.Bind("extensionSeen", func() int64 { return a.extensionSeen() })
	w.Bind("appVersion", func() string { return version.Version })
	w.Bind("updateInfo", func() updateInfo { return a.upd.info() })
	w.Bind("debugInfo", func() string { return a.debugInfo() })
	w.Bind("refreshLink", func(id int, u string) error { return a.refreshLink(id, u) })
	w.Bind("checkUpdate", func() { go a.upd.check(a) })
	w.Bind("installUpdate", func() error { return a.upd.install(a) })
	w.Bind("pasteText", func() string {
		t, _ := clipboardText()
		return strings.TrimSpace(t)
	})
	w.Bind("connections", func() int { return a.eng.SegmentsDefault() })
	w.Bind("appStatus", func() status { return a.status() })
	w.Bind("getSettings", func() Settings { return a.getSettings() })
	w.Bind("setSettings", func(s Settings) error { return a.setSettings(s) })
	w.Bind("setSpeedLimit", func(kb int) error { return a.setSpeedLimit(kb) })
	w.Bind("add", func(url string) (int, error) {
		return a.add(engine.Request{URL: url}), nil
	})
	w.Bind("pause", func(id int) error { return a.pause(id) })
	w.Bind("resume", func(id int) error { return a.resume(id) })
	w.Bind("remove", func(id int) error { return a.remove(id) })
	w.Bind("openFile", func(id int) error { return a.openFile(id, false) })
	w.Bind("showInFolder", func(id int) error { return a.openFile(id, true) })
	w.Bind("openDir", func() error { return a.openDir() })
	w.Bind("pauseAll", func() { a.all("pause") })
	w.Bind("clearDone", func() int { return a.clearDone() })
	w.Bind("resumeAll", func() { a.all("resume") })
	w.Bind("downloadDir", func() string { return a.eng.Dir() })
	w.Bind("defaultDir", func() string { return a.defaultDir })
	// Runs on the window's thread, so the picker is modal to it.
	w.Bind("pickFolder", func(start string) (string, error) {
		return pickFolder(uintptr(w.Window()), "Choose download folder", start)
	})
}
