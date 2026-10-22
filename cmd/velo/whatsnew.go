package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Adoxcol/velo"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

type releaseNotesView struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

func notesMarker() string { return filepath.Join(ipc.Dir(), "release-notes-seen") }

// Keep this outside Settings so an older settings form cannot undo dismissal.
func pendingReleaseNotes(path, current string, onboarded bool) (*releaseNotesView, error) {
	if current == "dev" || current == "" {
		return nil, nil
	}
	seen, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if strings.TrimSpace(string(seen)) == current {
		return nil, nil
	}
	if !onboarded {
		return nil, os.WriteFile(path, []byte(current), 0o600)
	}
	notes := velo.ReleaseNotes(current)
	if notes == "" {
		notes = "Velo has been updated. Release notes for this version are available at https://velo.adoxcol.com/changelog."
	}
	return &releaseNotesView{Version: current, Notes: notes}, nil
}

func (a *app) pendingReleaseNotes() (*releaseNotesView, error) {
	return pendingReleaseNotes(notesMarker(), version.Version, a.getSettings().Onboarded)
}

func dismissReleaseNotes() error {
	return os.WriteFile(notesMarker(), []byte(version.Version), 0o600)
}
