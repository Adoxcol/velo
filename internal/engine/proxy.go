package engine

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
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
	firstStar := strings.IndexByte(pattern, '*')
	if firstStar == -1 {
		return pattern == host
	}
	prefix := pattern[:firstStar]
	if !strings.HasPrefix(host, prefix) {
		return false
	}
	host = host[len(prefix):]
	pattern = pattern[firstStar+1:]

	for {
		nextStar := strings.IndexByte(pattern, '*')
		if nextStar == -1 {
			return strings.HasSuffix(host, pattern)
		}
		sub := pattern[:nextStar]
		idx := strings.Index(host, sub)
		if idx < 0 {
			return false
		}
		host = host[idx+len(sub):]
		pattern = pattern[nextStar+1:]
	}
}
