// Package hostreg registers Velo's native messaging host with Chromium
// browsers, so the extension can talk to Velo. Used by the installer and
// by portable copies.
package hostreg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	HostName = "com.velo.host"
	// The development extension ID (fixed by the "key" in extension/manifest.json).
	DevExtensionID = "dmdcijgljocjdmafenflenjhdjhenokc"
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
	for _, br := range Browsers {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\`+br+`\NativeMessagingHosts\`+HostName, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = k.SetStringValue("", manifest)
		k.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisteredTo reports whether browsers currently use the host in dir.
func RegisteredTo(dir string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Google\Chrome\NativeMessagingHosts\`+HostName, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return err == nil && strings.EqualFold(filepath.Clean(v), filepath.Clean(manifestPath(dir)))
}

// Unregister removes the host from every browser.
func Unregister() {
	for _, br := range Browsers {
		registry.DeleteKey(registry.CURRENT_USER, `Software\`+br+`\NativeMessagingHosts\`+HostName)
	}
}
