// Package hostreg registers Velo's native messaging host with Chromium
// browsers, so the extension can talk to Velo. Used by the installer and
// by portable copies.
package hostreg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

)

const (
	HostName = "com.velo.host"
	// The development extension ID (fixed by the "key" in extension/manifest.json).
	DevExtensionID = "dmdcijgljocjdmafenflenjhdjhenokc"
	// FirefoxID is the Firefox add-on ID (set by tools/packext).
	FirefoxID  = "velo@adoxcol.github.io"
	firefoxKey = `Software\Mozilla\NativeMessagingHosts\` + HostName
)

// Store extension IDs: fill in once Velo is published.
var (
	ChromeStoreID = ""
	EdgeStoreID   = ""
)

// Browsers that read native messaging hosts from HKCU\Software\<key>\NativeMessagingHosts.
var Browsers = []string{`Google\Chrome`, `Microsoft\Edge`, `BraveSoftware\Brave-Browser`, `Chromium`}

func manifestPath(dir string) string { return filepath.Join(dir, HostName+".json") }

// Register writes the host manifest into dir (next to velo-host.exe) and
// points every browser at it.
func Register(dir string) error {
	origins := []string{"chrome-extension://" + DevExtensionID + "/"}
	for _, id := range []string{ChromeStoreID, EdgeStoreID} {
		if id != "" {
			origins = append(origins, "chrome-extension://"+id+"/")
		}
	}
	manifest := manifestPath(dir)
	b, _ := json.MarshalIndent(map[string]any{
		"name":            HostName,
		"description":     "Velo native messaging host",
		"path":            filepath.Join(dir, "velo-host.exe"),
		"type":            "stdio",
		"allowed_origins": origins,
	}, "", "  ")
	if err := os.WriteFile(manifest, b, 0o644); err != nil {
		return err
	}
	// Firefox wants its own manifest: add-on IDs instead of origins.
	ffManifest := filepath.Join(dir, HostName+".firefox.json")
	fb, _ := json.MarshalIndent(map[string]any{
		"name":               HostName,
		"description":        "Velo native messaging host",
		"path":               filepath.Join(dir, "velo-host.exe"),
		"type":               "stdio",
		"allowed_extensions": []string{FirefoxID},
	}, "", "  ")
	if err := os.WriteFile(ffManifest, fb, 0o644); err != nil {
		return err
	}
	if err := registerKey(firefoxKey, ffManifest); err != nil {
		return err
	}
	for _, br := range Browsers {
		if err := registerKey(`Software\`+br+`\NativeMessagingHosts\`+HostName, manifest); err != nil {
			return err
		}
	}
	return nil
}

// RegisteredTo reports whether browsers currently use the host in dir.
func RegisteredTo(dir string) bool {
	v, err := readKey(`Software\Google\Chrome\NativeMessagingHosts\` + HostName)
	return err == nil && strings.EqualFold(filepath.Clean(v), filepath.Clean(manifestPath(dir)))
}

// Unregister removes the host from every browser.
func Unregister() {
	deleteKey(firefoxKey)
	for _, br := range Browsers {
		deleteKey(`Software\` + br + `\NativeMessagingHosts\` + HostName)
	}
}
