package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/update"
	"github.com/Adoxcol/velo/internal/version"
)

var defaultUpdateClient = sync.OnceValue(func() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.TLSHandshakeTimeout = 5 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second
	tr.MaxIdleConns = 10
	tr.IdleConnTimeout = 90 * time.Second
	return &http.Client{Transport: tr}
})

// updater keeps Velo up to date from GitHub Releases. With auto-update on
// (the default) a verified update is installed when Velo is idle: nothing
// downloading and the window closed. Otherwise the user is notified.
type updater struct {
	mu         sync.Mutex
	status     string // shown in Settings → Advanced
	busy       bool   // true when check or update download is running
	release    *update.Release
	downloaded string // verified installer, ready to run
	notified   string // version we already notified about
}

type updateInfo struct {
	Current   string `json:"current"`
	Status    string `json:"status"`
	Available string `json:"available"` // newer version, if any
	Ready     bool   `json:"ready"`     // downloaded and verified
	Page      string `json:"page"`
	Checking  bool   `json:"checking"`
}

func (u *updater) info() updateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	i := updateInfo{Current: version.Version, Status: u.status, Ready: u.downloaded != "", Checking: u.busy}
	if u.release != nil {
		i.Available, i.Page = u.release.Version, u.release.Page
	}
	return i
}

func (u *updater) set(status string) {
	u.mu.Lock()
	u.status = status
	u.mu.Unlock()
}

// check looks for a newer release and downloads it. Safe to call anytime.
func (u *updater) check(a *app) {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return
	}
	u.busy = true
	u.status = "Checking for updates…"
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	if version.Version == "dev" {
		u.set("Updates are off in development builds.")
		return
	}
	_, portable := ipc.Portable()
	// No overall time limit: update.Download resumes after interruptions and
	// only gives up when the connection stalls, so slow links still update.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	client := defaultUpdateClient()
	checkCtx, cancelCheck := context.WithTimeout(ctx, 15*time.Second)
	rel, err := update.Latest(checkCtx, client)
	cancelCheck()
	switch {
	case errors.Is(err, update.ErrNoRelease):
		u.set("No releases published yet.")
		return
	case err != nil:
		u.set("Couldn't check for updates: " + err.Error())
		log.Printf("update check: %v", err)
		return
	case !update.Newer(rel.Version, version.Version):
		u.set("Velo is up to date.")
		return
	}
	if portable { // the installer would install a second, normal copy
		u.mu.Lock()
		u.release = rel
		u.mu.Unlock()
		u.set("Velo " + rel.Version + " is out. This is a portable copy: download the new portable zip from GitHub.")
		return
	}
	u.mu.Lock()
	u.release = rel
	ready := u.downloaded != "" && filepath.Base(u.downloaded) == "Velo-Setup-"+rel.Version+".exe"
	u.mu.Unlock()
	if !ready {
		u.set("Downloading Velo " + rel.Version + "…")
		path, err := update.Download(ctx, client, rel, filepath.Join(os.TempDir(), "Velo-update"))
		if err != nil {
			u.set("Update download failed: " + err.Error())
			log.Printf("update download: %v", err)
			return
		}
		u.mu.Lock()
		u.downloaded = path
		u.mu.Unlock()
	}
	u.set("Velo " + rel.Version + " is ready to install.")
	log.Printf("update %s downloaded and verified", rel.Version)

	if a.getSettings().AutoUpdate && a.idle() {
		u.install(a)
		return
	}
	u.mu.Lock()
	first := u.notified != rel.Version
	u.notified = rel.Version
	u.mu.Unlock()
	if first {
		toast.Show(strings.ReplaceAll(a.tr("Velo {0} is ready"), "{0}", rel.Version), a.tr("Click to update now. Downloads resume afterwards."), func() { u.install(a) })
	}
}

// install runs the verified installer, which closes and restarts Velo.
func (u *updater) install(a *app) error {
	u.mu.Lock()
	path := u.downloaded
	u.mu.Unlock()
	if path == "" {
		return errors.New("no update has been downloaded yet")
	}
	log.Printf("installing update %s", path)
	a.saveHistory()
	return update.Install(path)
}

// idle: no downloads running and no window open, so updating won't interrupt.
func (a *app) idle() bool {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	return running == 0 && !hasMiniWindow() && !mainVisible()
}

// run checks a minute after start, then every 12 hours; a pending update is
// installed once Velo becomes idle.
func (u *updater) run(a *app) {
	time.Sleep(time.Minute)
	last := time.Time{}
	for {
		if a.getSettings().AutoUpdate {
			if time.Since(last) > 12*time.Hour {
				last = time.Now()
				u.check(a)
			} else if u.info().Ready && a.idle() {
				u.install(a)
				return
			}
		}
		time.Sleep(10 * time.Minute)
	}
}
