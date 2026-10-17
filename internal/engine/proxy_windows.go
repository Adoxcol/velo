package engine

import (
	"golang.org/x/sys/windows/registry"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The Windows proxy (Settings > Network > Proxy), re-read every 30 seconds.
// Automatic setup scripts (PAC) aren't supported: those connect directly.
var sysProxy struct {
	sync.Mutex
	read             time.Time
	enabled          bool
	servers          map[string]string // scheme ("" = all) -> host:port
	bypass           []string
	bypassLocalNames bool
}

func systemProxy(target *url.URL) *url.URL {
	sysProxy.Lock()
	defer sysProxy.Unlock()
	if time.Since(sysProxy.read) > 30*time.Second {
		sysProxy.read = time.Now()
		readSystemProxy()
	}
	if !sysProxy.enabled {
		return nil
	}
	host := strings.ToLower(target.Hostname())
	if sysProxy.bypassLocalNames && !strings.Contains(host, ".") {
		return nil
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	for _, b := range sysProxy.bypass {
		if wildcardMatch(b, host) {
			return nil
		}
	}
	server := sysProxy.servers[target.Scheme]
	if server == "" {
		server = sysProxy.servers[""]
	}
	if server == "" {
		return nil
	}
	scheme := "http"
	if s := sysProxy.servers["socks"]; s != "" && server == s {
		scheme = "socks5"
	}
	u, err := url.Parse(scheme + "://" + server)
	if err != nil {
		return nil
	}
	return u
}

func readSystemProxy() {
	sysProxy.enabled, sysProxy.servers, sysProxy.bypass, sysProxy.bypassLocalNames = false, nil, nil, false
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	if server == "" {
		return
	}
	sysProxy.enabled = true
	sysProxy.servers = parseProxyServer(server)
	override, _, _ := k.GetStringValue("ProxyOverride")
	for _, b := range strings.Split(override, ";") {
		b = strings.ToLower(strings.TrimSpace(b))
		switch b {
		case "":
		case "<local>":
			sysProxy.bypassLocalNames = true
		default:
			sysProxy.bypass = append(sysProxy.bypass, b)
		}
	}
}
