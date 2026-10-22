package velo

import (
	"strings"
	"testing"
)

func TestReleaseNotesExactSection(t *testing.T) {
	notes := ReleaseNotes("0.8.11")
	if !strings.Contains(notes, "Offer to download copied links") || strings.Contains(notes, "Rename videos") || strings.Contains(notes, "## 0.8.") {
		t.Fatalf("wrong release section: %q", notes)
	}
	for _, v := range []string{"dev", "0.8.", "99.0.0", ""} {
		if got := ReleaseNotes(v); got != "" {
			t.Errorf("ReleaseNotes(%q) = %q", v, got)
		}
	}
}
