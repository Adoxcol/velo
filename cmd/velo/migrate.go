package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"velo/internal/ipc"
)

// migrateFromMyDM carries settings and history over from the app's old name
// (MyDM, data in %APPDATA%\mydm) the first time Velo runs. The old download
// folder is kept so existing and paused downloads stay where they are.
func migrateFromMyDM() {
	cfg, err := os.UserConfigDir()
	if err != nil || os.Getenv("VELO_HOME") != "" {
		return
	}
	oldDir := filepath.Join(cfg, "mydm")
	if _, err := os.Stat(filepath.Join(ipc.Dir(), "settings.json")); err == nil {
		return // already set up
	}
	if _, err := os.Stat(oldDir); err != nil {
		return // nothing to migrate
	}
	os.MkdirAll(ipc.Dir(), 0o700)
	for _, name := range []string{"settings.json", "history.json"} {
		if b, err := os.ReadFile(filepath.Join(oldDir, name)); err == nil {
			os.WriteFile(filepath.Join(ipc.Dir(), name), b, 0o644)
		}
	}
	// Keep using the old default folder if the user never chose one.
	s := loadSettings()
	if s.DownloadDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			old := filepath.Join(home, "Downloads", "MyDM")
			if fi, err := os.Stat(old); err == nil && fi.IsDir() {
				s.DownloadDir = old
			}
		}
	}
	// Replace the old start-with-Windows entry.
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if _, _, err := k.GetStringValue("MyDM"); err == nil {
			k.DeleteValue("MyDM")
			s.Autostart = true
			setAutostart(true)
		}
		k.Close()
	}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		os.WriteFile(settingsPath(), b, 0o644)
	}
	log.Printf("migrated settings and history from %s", oldDir)
}
