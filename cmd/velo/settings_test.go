package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func at(day time.Weekday, hhmm string) time.Time {
	m, _ := minutes(hhmm)
	// 2026-10-11 is a Sunday.
	return time.Date(2026, 10, 11+int(day), m/60, m%60, 0, 0, time.Local)
}

func TestScheduleAllowed(t *testing.T) {
	all := [7]bool{true, true, true, true, true, true, true}
	weekdays := [7]bool{false, true, true, true, true, true, false}
	cases := []struct {
		name string
		s    Schedule
		t    time.Time
		want bool
	}{
		{"disabled", Schedule{Start: "01:00", Stop: "02:00"}, at(time.Monday, "12:00"), true},
		{"inside day window", Schedule{true, "09:00", "17:00", all}, at(time.Monday, "09:00"), true},
		{"stop is exclusive", Schedule{true, "09:00", "17:00", all}, at(time.Monday, "17:00"), false},
		{"before window", Schedule{true, "09:00", "17:00", all}, at(time.Monday, "08:59"), false},
		{"day not selected", Schedule{true, "09:00", "17:00", weekdays}, at(time.Sunday, "10:00"), false},
		{"overnight, evening part", Schedule{true, "23:00", "06:00", all}, at(time.Monday, "23:30"), true},
		{"overnight, morning part", Schedule{true, "23:00", "06:00", all}, at(time.Tuesday, "05:59"), true},
		{"overnight, midday", Schedule{true, "23:00", "06:00", all}, at(time.Tuesday, "12:00"), false},
		// Friday 23:00 window runs into Saturday morning even though Saturday is off.
		{"overnight carries into unselected day", Schedule{true, "23:00", "06:00", weekdays}, at(time.Saturday, "02:00"), true},
		{"overnight from unselected day", Schedule{true, "23:00", "06:00", weekdays}, at(time.Monday, "02:00"), false},
		{"start == stop is all day", Schedule{true, "00:00", "00:00", weekdays}, at(time.Wednesday, "15:00"), true},
	}
	for _, c := range cases {
		if got := c.s.allowed(c.t); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidateDownloadDir(t *testing.T) {
	s := defaultSettings()
	s.DownloadDir = "relative/path"
	if err := s.validate(); err == nil {
		t.Fatal("expected error for relative DownloadDir, got nil")
	}

	s = defaultSettings()
	cleanDir := filepath.Join(t.TempDir(), "sub", "..", "clean")
	s.DownloadDir = "  " + cleanDir + "  "
	if err := s.validate(); err != nil {
		t.Fatalf("validate failed for absolute DownloadDir: %v", err)
	}
	if s.DownloadDir != filepath.Clean(cleanDir) {
		t.Fatalf("expected cleaned DownloadDir %q, got %q", filepath.Clean(cleanDir), s.DownloadDir)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VELO_HOME", dir)

	s := defaultSettings()
	if err := s.save(); err != nil {
		t.Fatalf("save settings failed: %v", err)
	}

	a := &app{home: dir, lastHistory: []byte("old")}
	a.saveHistory()

	if runtime.GOOS == "windows" {
		return
	}

	for _, name := range []string{"settings.json", "history.json"} {
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", name, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s mode = %o, want 0600", name, perm)
		}
	}
}

func TestValidate(t *testing.T) {
	s := defaultSettings()
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.MaxConcurrent = 0 },
		func(s *Settings) { s.SpeedLimitKB = -1 },
		func(s *Settings) { s.Schedule.Start = "25:00" },
		func(s *Settings) { s.Schedule.Stop = "7am" },
		func(s *Settings) { s.OnFinish = "reboot" },
	} {
		x := defaultSettings()
		bad(&x)
		if x.validate() == nil {
			t.Errorf("expected error for %+v", x)
		}
	}
}

func TestCopiedLink(t *testing.T) {
	for text, want := range map[string]string{
		"https://example.com/files/setup.exe":        "setup.exe",
		"  https://example.com/a/movie.mkv?sig=1 \n": "movie.mkv",
		"https://example.com/live/master.m3u8":       "master.m3u8",
		"https://example.com/article.html":           "",
		"check this https://example.com/a.zip":       "",
		"just some text":                             "",
		`C:\Users\me\a.zip`:                          "",
	} {
		u, _, ok := copiedLink(text)
		got := ""
		if ok {
			got = linkName(u)
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}
