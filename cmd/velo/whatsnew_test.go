package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseNotesLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen")
	check := func(v string, onboarded, want bool) {
		t.Helper()
		got, err := pendingReleaseNotes(path, v, onboarded)
		if err != nil || (got != nil) != want {
			t.Fatalf("pending(%s, %t) = %v, %v; want pending %t", v, onboarded, got, err, want)
		}
		if got != nil && (got.Version != v || got.Notes == "") {
			t.Fatalf("invalid notes: %+v", got)
		}
	}
	check("dev", false, false)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("development build wrote a marker")
	}
	check("0.8.10", false, false) // fresh install: onboarding only
	check("0.8.10", true, false)
	check("0.8.11", true, true) // automatic or manual update
	check("0.8.11", true, true) // closing the window does not dismiss notes
	if err := os.WriteFile(path, []byte("0.8.11"), 0o600); err != nil {
		t.Fatal(err)
	}
	check("0.8.11", true, false)
	check("0.8.12", true, true)
	check("99.0.0", true, true) // missing bundled section has a fallback
}

func TestReleaseNotesExistingInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen")
	got, err := pendingReleaseNotes(path, "0.8.12", true)
	if err != nil || got == nil {
		t.Fatalf("existing users should see notes on rollout: %v, %v", got, err)
	}
}
