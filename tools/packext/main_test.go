package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestBrowserPolicyPackaging(t *testing.T) {
	for _, browser := range []string{"chrome", "firefox", "personal"} {
		var out bytes.Buffer
		if err := writeManifest(&out, filepath.Join("..", "..", "extension", "manifest.json"), "0.8.21", browser); err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Key     string         `json:"key"`
			Gecko   map[string]any `json:"browser_specific_settings"`
			Content []struct {
				JS      []string `json:"js"`
				Exclude []string `json:"exclude_matches"`
			} `json:"content_scripts"`
			Background struct {
				Scripts []string `json:"scripts"`
				Worker  string   `json:"service_worker"`
			} `json:"background"`
		}
		if err := json.Unmarshal(out.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if len(manifest.Content) != 1 || manifest.Content[0].JS[0] != "policy.js" {
			t.Fatal("policy must load before overlays")
		}
		switch browser {
		case "firefox":
			if manifest.Gecko["gecko"] == nil || len(manifest.Content[0].Exclude) != 0 || manifest.Background.Scripts[0] != "policy.js" {
				t.Fatal("incorrect Firefox policy packaging")
			}
		case "chrome":
			if manifest.Gecko != nil || len(manifest.Content[0].Exclude) == 0 || manifest.Background.Worker != "background.js" || manifest.Key != "" {
				t.Fatal("Chrome must retain restrictions")
			}
		case "personal":
			if manifest.Gecko != nil || len(manifest.Content[0].Exclude) != 0 || manifest.Background.Worker != "background.js" || manifest.Key == "" {
				t.Fatal("the personal build drops restrictions and keeps the development ID")
			}
		}
	}
}
