// packext builds the browser extension .zip for the Chrome Web Store and
// Microsoft Edge Add-ons. The stores assign their own extension ID, so the
// development "key" is removed from the manifest. Run from the repo root:
//
//	go run ./tools/packext -version 0.1.0 -out dist/velo-extension-0.1.0.zip
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
)

func main() {
	ver := flag.String("version", "", "extension version (e.g. 0.1.0); default: keep manifest version")
	out := flag.String("out", "dist/velo-extension.zip", "output .zip")
	src := flag.String("src", "extension", "extension folder")
	flag.Parse()

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
		w, err := zw.CreateHeader(&zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		if rel == "manifest.json" {
			return writeManifest(w, p, *ver)
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

func writeManifest(w io.Writer, path, ver string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	delete(m, "key") // the store assigns the ID
	if ver != "" {
		m["version"] = ver
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}
