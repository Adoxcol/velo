package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"velo/internal/engine"
)

// The download list is saved to %APPDATA%\velo\history.json so finished,
// failed and queued downloads survive a restart. Paused downloads with saved
// progress are restored from their .part.json files instead (engine.FindPaused).

type histEntry struct {
	Req      engine.Request `json:"req"`
	Path     string         `json:"path,omitempty"`
	Total    int64          `json:"total"`
	Status   string         `json:"status"`
	Added    int64          `json:"added,omitempty"`
	Finished int64          `json:"finished,omitempty"`
	Error    string         `json:"error,omitempty"`
	Verified bool           `json:"verified,omitempty"`
}

const maxHistory = 500

// historyPath is fixed when the app starts, so background work can't write to
// a different folder later (e.g. after a test restores VELO_HOME).
func (a *app) historyPath() string { return filepath.Join(a.home, "history.json") }

func (a *app) snapshot() []histEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []histEntry
	for _, it := range a.items {
		d := it.d
		status, err := d.State()
		p, total := d.Info()
		if status == "downloading" || status == "verifying" {
			status = "queued" // was interrupted: start again on the next run
		}
		e := histEntry{Req: d.Req, Path: p, Total: total, Status: status, Added: it.added.UnixMilli()}
		if status == "done" {
			e.Verified = d.Verified
			e.Req.Cookies, e.Req.Pass = "", "" // only needed to finish the download
		}
		if !it.finished.IsZero() {
			e.Finished = it.finished.UnixMilli()
		}
		if err != nil {
			e.Error = err.Error()
		}
		out = append(out, e)
	}
	if len(out) > maxHistory {
		out = out[len(out)-maxHistory:]
	}
	return out
}

// saveHistory writes the list if it changed since the last save.
func (a *app) saveHistory() {
	b, err := json.MarshalIndent(a.snapshot(), "", " ")
	if err != nil {
		return
	}
	a.mu.Lock()
	same := bytes.Equal(b, a.lastHistory)
	a.lastHistory = b
	a.mu.Unlock()
	if same {
		return
	}
	tmp := a.historyPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err == nil {
		os.Rename(tmp, a.historyPath())
	}
}

func (a *app) readHistory() []histEntry {
	b, err := os.ReadFile(a.historyPath())
	if err != nil {
		return nil
	}
	var entries []histEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		log.Printf("history: %v", err)
		return nil
	}
	return entries
}

// restore rebuilds the download list at startup: saved history plus paused
// downloads found in any folder that history mentions (the download folder
// may have changed since they started) or the current folder.
func (a *app) restore() {
	entries := a.readHistory()
	dirs := []string{}
	for _, d := range []string{a.eng.Dir(), a.defaultDir} {
		dirs = append(dirs, d)
		for _, c := range engine.Categories {
			dirs = append(dirs, filepath.Join(d, c))
		}
	}
	for _, e := range entries {
		if e.Path != "" {
			dirs = append(dirs, filepath.Dir(e.Path))
		}
	}
	var paused []*engine.Download
	seenDir := map[string]bool{}
	skip := map[string]bool{}
	for _, d := range dirs {
		key := strings.ToLower(filepath.Clean(d))
		if seenDir[key] {
			continue
		}
		seenDir[key] = true
		for _, p := range engine.FindPaused(d) {
			paused = append(paused, p)
			skip[p.Path] = true
		}
	}
	a.loadHistory(entries, skip)
	// .part.json files don't keep logins; history does.
	logins := map[string]engine.Request{}
	for _, e := range entries {
		if e.Path != "" {
			logins[e.Path] = e.Req
		}
	}
	for _, d := range paused {
		if r, ok := logins[d.Path]; ok {
			d.Req.Cookies, d.Req.User, d.Req.Pass = r.Cookies, r.User, r.Pass
		}
		a.register(d)
		log.Printf("#%d paused at %.1f%%: %s", d.ID, pct(d), d.Path)
	}
}

// loadHistory registers saved entries. Paths in skip (paused downloads
// restored from their own state files) are left out to avoid duplicates.
func (a *app) loadHistory(entries []histEntry, skip map[string]bool) {
	for _, e := range entries {
		if e.Path != "" && skip[e.Path] {
			continue
		}
		d := &engine.Download{Req: e.Req, Total: -1}
		switch e.Status {
		case "done":
			d.Path, d.Total, d.Status, d.Verified = e.Path, e.Total, "done", e.Verified
			d.Done.Store(max(e.Total, 0))
		case "error":
			d.Status, d.Err = "error", errors.New(e.Error)
		case "queued":
			d.Status = "queued"
			dropStalePart(e.Path)
		default: // paused before any progress was saved: starts over on resume
			d.Status = "paused"
			dropStalePart(e.Path)
		}
		it := a.register(d)
		if e.Added > 0 {
			it.added = time.UnixMilli(e.Added)
		}
		if e.Finished > 0 {
			it.finished = time.UnixMilli(e.Finished)
		}
	}
}

// clearDone removes finished downloads from the list (files are kept).
func (a *app) clearDone() int {
	a.mu.Lock()
	kept := a.items[:0]
	n := 0
	for _, it := range a.items {
		if s, _ := it.d.State(); s == "done" {
			n++
			if a.byID != nil {
				delete(a.byID, it.d.ID)
			}
			continue
		}
		kept = append(kept, it)
	}
	a.items = kept
	a.mu.Unlock()
	a.kick()
	return n
}

// dropStalePart deletes the partial file of a download that will start over
// (Velo was closed while it ran and it can't be resumed), so the restart
// reuses the original name instead of saving as "name (1)".
func dropStalePart(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path + ".part.json"); err == nil {
		return // resumable: its own state file restores it
	}
	os.Remove(path + ".part")
	os.RemoveAll(path + ".parts")
}
