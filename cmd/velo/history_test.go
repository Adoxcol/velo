package main

import (
	"os"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
)

func TestHistEqual(t *testing.T) {
	e1 := histEntry{
		Req:    engine.Request{URL: "https://example.com/1.bin"},
		Path:   "/tmp/1.bin",
		Total:  1000,
		Status: "done",
	}
	e2 := histEntry{
		Req:    engine.Request{URL: "https://example.com/2.bin"},
		Path:   "/tmp/2.bin",
		Total:  2000,
		Status: "queued",
	}

	snap1 := []histEntry{e1, e2}
	snap2 := []histEntry{e1, e2}
	snap3 := []histEntry{e1}

	if !histEqual(snap1, snap2) {
		t.Errorf("histEqual(snap1, snap2) = false, want true")
	}
	if histEqual(snap1, snap3) {
		t.Errorf("histEqual(snap1, snap3) = true, want false")
	}

	e2Modified := e2
	e2Modified.Status = "done"
	snap4 := []histEntry{e1, e2Modified}
	if histEqual(snap1, snap4) {
		t.Errorf("histEqual(snap1, snap4) = true, want false")
	}
}

func TestSaveHistoryOptimization(t *testing.T) {
	home := t.TempDir()
	a := &app{
		home: home,
	}

	d := &engine.Download{
		ID:    1,
		Req:   engine.Request{URL: "https://example.com/file.zip"},
		Path:  "/downloads/file.zip",
		Total: 1024 * 1024,
	}
	d.SetStatus("done")
	it := &item{
		d:        d,
		added:    time.Now(),
		finished: time.Now(),
	}
	a.items = append(a.items, it)

	// First call saves to disk and populates lastSnap.
	a.saveHistory()

	if len(a.lastSnap) != 1 {
		t.Fatalf("expected 1 item in lastSnap, got %d", len(a.lastSnap))
	}
	firstJSON := append([]byte(nil), a.lastHistory...)

	// Statutory check: file exists on disk.
	if _, err := os.Stat(a.historyPath()); err != nil {
		t.Fatalf("history file missing: %v", err)
	}

	// Second call with unchanged items skips re-marshaling and file write.
	a.saveHistory()
	if string(a.lastHistory) != string(firstJSON) {
		t.Fatalf("lastHistory changed when nothing changed")
	}

	// Modifying status triggers an update.
	d.SetStatus("error")
	a.saveHistory()
	if a.lastSnap[0].Status != "error" {
		t.Fatalf("expected status 'error' in lastSnap, got %s", a.lastSnap[0].Status)
	}
}
