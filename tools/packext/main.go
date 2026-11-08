// packext builds the browser extension .zip for the Chrome Web Store and
// Microsoft Edge Add-ons, or (-browser firefox) for Firefox Add-ons. The
// Chrome stores assign their own extension ID, so the development "key" is
// removed from the manifest. Run from the repo root:
//
//	go run ./tools/packext -version 0.1.0 -out dist/velo-extension-0.1.0.zip
//	go run ./tools/packext -browser firefox -out dist/velo-firefox-0.1.0.zip
//
// -browser personal builds a Chrome extension without the Chrome Web Store
// restrictions (YouTube included), for loading unpacked on your own PC only.
// It keeps the development key, so its ID is the one Velo always allows.
// Never upload it to a store.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adoxcol/velo/internal/hostreg"
)

func main() {
	ver := flag.String("version", "", "extension version (e.g. 0.1.0); default: keep manifest version")
	out := flag.String("out", "dist/velo-extension.zip", "output .zip")
	src := flag.String("src", "extension", "extension folder")
	browser := flag.String("browser", "chrome", "chrome (also Edge, Brave), firefox, or personal (unrestricted; never upload it)")
	flag.Parse()
	if *browser != "chrome" && *browser != "firefox" && *browser != "personal" {
		log.Fatalf("unknown -browser %q", *browser)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(*src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(*src, p)
		rel = filepath.ToSlash(rel)
		if rel == "EXTENSION_ID.txt" || strings.HasPrefix(filepath.Base(rel), ".") {
			return nil // development-only files
		}
		if rel == "youtube.js" && *browser == "chrome" {
			return nil // the Chrome Web Store build doesn't support YouTube
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		if rel == "manifest.json" {
			return writeManifest(w, p, *ver, *browser)
		}
		if rel == "policy.js" && *browser == "personal" {
			_, err := io.WriteString(w, personalPolicy)
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", *out)
}

// personalPolicy replaces policy.js in the personal build: nothing is
// blocked, and the service worker loads the YouTube page matching.
const personalPolicy = `// Personal build (tools/packext -browser personal): no store restrictions.
globalThis.veloPolicy = Object.freeze({ blocked: () => false, message: "" });
if (typeof importScripts === "function") importScripts("youtube.js");
`

func writeManifest(w io.Writer, path, ver, browser string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if browser != "personal" {
		delete(m, "key") // the store assigns the ID
	}
	if ver != "" {
		m["version"] = ver
	}
	if browser != "chrome" {
		for _, script := range m["content_scripts"].([]any) {
			delete(script.(map[string]any), "exclude_matches")
		}
	}
	if browser == "personal" {
		m["name"] = m["name"].(string) + " (personal)"
	}
	if browser == "firefox" {
		// Firefox runs background scripts as an event page, not a service
		// worker, and needs a fixed add-on ID for native messaging.
		m["background"] = map[string]any{"scripts": []string{"policy.js", "youtube.js", "i18n.js", "background.js"}}
		m["browser_specific_settings"] = map[string]any{"gecko": map[string]any{
			"id":                          hostreg.FirefoxID,
			"strict_min_version":          "142.0",
			"data_collection_permissions": map[string]any{"required": []string{"none"}},
		}}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}
