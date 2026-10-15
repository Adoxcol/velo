package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// ErrStalled means a connection received no data for too long; callers
// retry it like any other network error.
var ErrStalled = errors.New("connection stalled (no data received)")

// newClient returns an HTTP client that gives up on servers that don't
// answer, and drops connections that stop sending data mid-download.
func newClient(idle time.Duration) *http.Client {
	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   32,
		// HTTP/1.1 only: HTTP/2 would put every segment on one TCP connection,
		// and servers that cap per-connection speed (GitHub, CDNs) cap it all.
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	return &http.Client{Transport: &idleTransport{base: base, idle: idle}}
}

type idleTransport struct {
	base http.RoundTripper
	idle time.Duration
}

func (t *idleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	b := &idleBody{rc: resp.Body, cancel: cancel, idle: t.idle}
	b.timer = time.AfterFunc(t.idle, func() { b.stalled.Store(true); cancel() })
	resp.Body = b
	return resp, nil
}

// idleBody cancels the request when no Read returns data within idle.
type idleBody struct {
	rc      io.ReadCloser
	cancel  context.CancelFunc
	idle    time.Duration
	timer   *time.Timer
	stalled atomic.Bool
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.timer.Reset(b.idle)
	}
	if err != nil && err != io.EOF && b.stalled.Load() {
		err = ErrStalled
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	b.cancel()
	return b.rc.Close()
}
