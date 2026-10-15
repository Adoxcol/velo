package engine

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Proxy modes: "" or "system" follows the Windows proxy setting (like
// browsers do), "none" connects directly, anything else is a proxy URL such
// as http://host:8080 or socks5://host:1080.

// ParseProxy checks a proxy setting and returns it normalised.
func ParseProxy(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "system":
		return "", nil
	case "none":
		return "none", nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Port() == "" {
		return "", errors.New("proxy must look like host:port, http://host:port or socks5://host:port")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", errors.New("proxy must be http://, https:// or socks5://")
	}
	return u.String(), nil
}

// SetProxy changes the proxy for new connections (see ParseProxy).
func (e *Engine) SetProxy(s string) {
	e.proxy.Store(&s)
	if t, ok := e.Client.Transport.(*idleTransport); ok {
		if b, ok := t.base.(*http.Transport); ok {
			b.CloseIdleConnections() // don't reuse connections made the old way
		}
	}
}

func (e *Engine) proxyFor(req *http.Request) (*url.URL, error) {
	mode := ""
	if p := e.proxy.Load(); p != nil {
		mode = *p
	}
	switch mode {
	case "none":
		return nil, nil
	case "", "system":
		if u := systemProxy(req.URL); u != nil {
			return u, nil
		}
		return http.ProxyFromEnvironment(req)
	}
	return url.Parse(mode)
}

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

// parseProxyServer reads "host:port" or "http=h:p;https=h:p;socks=h:p".
func parseProxyServer(s string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			m[strings.ToLower(k)] = v
		} else {
			m[""] = part
		}
	}
	if m[""] == "" && m["socks"] != "" && m["http"] == "" && m["https"] == "" {
		m[""] = m["socks"]
	}
	return m
}

// wildcardMatch matches Windows bypass entries like "*.corp.com" or "10.*".
func wildcardMatch(pattern, host string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == host
	}
	if !strings.HasPrefix(host, parts[0]) {
		return false
	}
	host = host[len(parts[0]):]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(host, p)
		if i < 0 {
			return false
		}
		host = host[i+len(p):]
	}
	return strings.HasSuffix(host, parts[len(parts)-1])
}
