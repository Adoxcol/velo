package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"velo/internal/engine"

	"velo/internal/ipc"
)

// Settings is saved to %APPDATA%\velo\settings.json.
type Settings struct {
	MaxConcurrent  int      `json:"maxConcurrent"` // downloads at once, 1-10
	SpeedLimitKB   int      `json:"speedLimitKB"`  // KB/s for all downloads, 0 = unlimited
	Schedule       Schedule `json:"schedule"`
	OnFinish       string   `json:"onFinish"`          // none, exit, sleep, shutdown
	DownloadDir    string   `json:"downloadDir"`       // "" = the default (Downloads\Velo)
	SortByType     bool     `json:"sortByType"`        // Video/Music/Documents/... subfolders
	Autostart      bool     `json:"autostart"`         // start with Windows (in the tray)
	Notify         bool     `json:"notify"`            // notification when a download finishes or fails
	WatchClipboard bool     `json:"watchClipboard"`    // offer to download copied file links
	Theme          string   `json:"theme"`             // light, dark or system
	Connections    int      `json:"connections"`       // default connections per download, 1-32
	ShowMini       bool     `json:"showMini"`          // floating Velo Mini window while downloading
	Onboarded      bool     `json:"onboarded"`         // first-run setup done
	AutoUpdate     bool     `json:"autoUpdate"`        // install verified updates when idle
	AskOnCapture   bool     `json:"askOnCapture"`      // show the download dialog for browser downloads
	Queues         []Queue  `json:"queues"`            // named queues besides the main one
	Proxy          string   `json:"proxy"`             // "" = Windows setting, "none", or a proxy URL
	MiniPos        *[2]int  `json:"miniPos,omitempty"` // where Velo Mini was dragged to (nil = bottom right)
}

// Schedule restricts downloading to a daily time window.
// Queue is a named queue with its own schedule and limit. Downloads not in
// a named queue use the main queue (Settings.Schedule, Settings.MaxConcurrent).
type Queue struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Schedule      Schedule `json:"schedule"`
	MaxConcurrent int      `json:"maxConcurrent"` // 0 = only the overall limit
}

type Schedule struct {
	Enabled bool    `json:"enabled"`
	Start   string  `json:"start"` // "HH:MM"
	Stop    string  `json:"stop"`  // "HH:MM"; before Start = overnight window
	Days    [7]bool `json:"days"`  // Sunday = 0, like time.Weekday
}

func defaultSettings() Settings {
	return Settings{
		MaxConcurrent:  3,
		OnFinish:       "none",
		SortByType:     true,
		Notify:         true,
		WatchClipboard: true,
		Theme:          "light",
		Connections:    16,
		ShowMini:       true,
		AutoUpdate:     true,
		AskOnCapture:   true,
		Schedule:       Schedule{Start: "01:00", Stop: "07:00", Days: [7]bool{true, true, true, true, true, true, true}},
	}
}

func settingsPath() string { return filepath.Join(ipc.Dir(), "settings.json") }

func loadSettings() Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.validate() != nil {
		return defaultSettings()
	}
	return s
}

func (s Settings) save() error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(settingsPath(), b, 0o644)
}

func (s *Settings) validate() error {
	if s.MaxConcurrent < 1 || s.MaxConcurrent > 10 {
		return fmt.Errorf("downloads at once must be 1-10")
	}
	if s.SpeedLimitKB < 0 {
		return fmt.Errorf("speed limit can't be negative")
	}
	if _, err := minutes(s.Schedule.Start); err != nil {
		return fmt.Errorf("start time: %w", err)
	}
	if _, err := minutes(s.Schedule.Stop); err != nil {
		return fmt.Errorf("stop time: %w", err)
	}
	switch s.OnFinish {
	case "", "none", "exit", "sleep", "shutdown":
	default:
		return fmt.Errorf("unknown finish action %q", s.OnFinish)
	}
	switch s.Theme {
	case "", "light", "dark", "system":
	default:
		return fmt.Errorf("unknown theme %q", s.Theme)
	}
	if s.Connections < 0 || s.Connections > 32 {
		return fmt.Errorf("connections must be 1-32")
	}
	seen := map[string]bool{}
	for i := range s.Queues {
		q := &s.Queues[i]
		q.Name = strings.TrimSpace(q.Name)
		if q.Name == "" || len(q.Name) > 40 {
			return fmt.Errorf("queue names must be 1-40 characters")
		}
		if q.ID == "" || seen[q.ID] {
			return fmt.Errorf("queue %q has a missing or duplicate ID", q.Name)
		}
		seen[q.ID] = true
		if q.MaxConcurrent < 0 || q.MaxConcurrent > 10 {
			return fmt.Errorf("queue %q: downloads at once must be 0-10", q.Name)
		}
		if _, err := minutes(q.Schedule.Start); err != nil {
			return fmt.Errorf("queue %q start time: %w", q.Name, err)
		}
		if _, err := minutes(q.Schedule.Stop); err != nil {
			return fmt.Errorf("queue %q stop time: %w", q.Name, err)
		}
	}
	if len(s.Queues) > 20 {
		return fmt.Errorf("at most 20 queues")
	}
	p, err := engine.ParseProxy(s.Proxy)
	if err != nil {
		return err
	}
	s.Proxy = p
	if s.DownloadDir != "" && !filepath.IsAbs(s.DownloadDir) {
		return fmt.Errorf(`download folder must be a full path like C:\Downloads`)
	}
	return nil
}

// minutes parses "HH:MM" into minutes since midnight.
func minutes(hhmm string) (int, error) {
	h, m, ok := strings.Cut(hhmm, ":")
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hi < 0 || hi > 23 || mi < 0 || mi > 59 {
		return 0, fmt.Errorf("%q is not HH:MM", hhmm)
	}
	return hi*60 + mi, nil
}

// allowed reports whether downloads may run at time t. An overnight window
// (e.g. 23:00-06:00) belongs to the day it starts on.
func (s Schedule) allowed(t time.Time) bool {
	if !s.Enabled {
		return true
	}
	start, _ := minutes(s.Start)
	stop, _ := minutes(s.Stop)
	now := t.Hour()*60 + t.Minute()
	day := t.Weekday()
	switch {
	case start == stop: // all day
		return s.Days[day]
	case start < stop:
		return s.Days[day] && now >= start && now < stop
	default: // overnight
		if now >= start {
			return s.Days[day]
		}
		return now < stop && s.Days[(day+6)%7]
	}
}

// describe is a short human summary for the window footer.
func (s Settings) describe(now time.Time) string {
	var parts []string
	if s.SpeedLimitKB > 0 {
		parts = append(parts, fmt.Sprintf("Limit %s/s", sizeKB(s.SpeedLimitKB)))
	}
	if s.Schedule.Enabled {
		state := "waiting"
		if s.Schedule.allowed(now) {
			state = "active"
		}
		parts = append(parts, fmt.Sprintf("Schedule %s–%s (%s)", s.Schedule.Start, s.Schedule.Stop, state))
	}
	switch s.OnFinish {
	case "exit":
		parts = append(parts, "Exit when done")
	case "sleep":
		parts = append(parts, "Sleep when done")
	case "shutdown":
		parts = append(parts, "Shut down when done")
	}
	return strings.Join(parts, " · ")
}

func sizeKB(kb int) string {
	if kb >= 1024 && kb%1024 == 0 {
		return fmt.Sprintf("%d MB", kb/1024)
	}
	if kb >= 1024 {
		return fmt.Sprintf("%.1f MB", float64(kb)/1024)
	}
	return fmt.Sprintf("%d KB", kb)
}
