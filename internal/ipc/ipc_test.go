package ipc

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	t1 := NewToken()
	t2 := NewToken()
	if len(t1) != 48 { // 24 bytes hex encoded = 48 chars
		t.Fatalf("expected 48 hex chars token, got len %d (%s)", len(t1), t1)
	}
	if t1 == t2 {
		t.Fatalf("generated identical tokens: %s", t1)
	}
}

func TestEndpointReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("VELO_HOME", tmpDir)

	ep := Endpoint{
		Port:  12345,
		Token: "test-token-12345",
		PID:   9999,
	}

	if err := WriteEndpoint(ep); err != nil {
		t.Fatalf("failed to write endpoint: %v", err)
	}

	// Verify file permissions (on POSIX systems)
	p := endpointPath()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("failed to stat endpoint file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("expected endpoint file permission 0600, got %o", perm)
		}
	}

	// Verify reading
	readEP, err := ReadEndpoint()
	if err != nil {
		t.Fatalf("failed to read endpoint: %v", err)
	}
	if readEP != ep {
		t.Errorf("read endpoint mismatch: got %+v, want %+v", readEP, ep)
	}

	RemoveEndpoint()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("expected endpoint file to be removed, but stat error was: %v", err)
	}
}

func TestIPCCallAuthentication(t *testing.T) {
	token := NewToken()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqToken := r.Header.Get(TokenHeader)
		if reqToken != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"id":1}`))
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Set up temporary VELO_HOME
	tmpDir := t.TempDir()
	t.Setenv("VELO_HOME", tmpDir)

	// Extract port from srv.Listener
	host := srv.Listener.Addr().String()
	var port int
	_, err := fmt.Sscanf(host[strings.LastIndex(host, ":")+1:], "%d", &port)
	if err != nil {
		t.Fatalf("failed to parse port from %s: %v", host, err)
	}

	ep := Endpoint{
		Port:  port,
		Token: token,
		PID:   os.Getpid(),
	}
	if err := WriteEndpoint(ep); err != nil {
		t.Fatalf("failed to write endpoint: %v", err)
	}

	// Test authorized call via Call()
	res, err := Call("ping", "", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if !bytes.Contains(res, []byte(`"ok":true`)) {
		t.Errorf("unexpected response: %s", string(res))
	}

	// Test unauthorized request (wrong token)
	unauthReq, _ := http.NewRequest("POST", srv.URL+"/ping", nil)
	unauthReq.Header.Set(TokenHeader, "wrong-token")
	resp, err := http.DefaultClient.Do(unauthReq)
	if err != nil {
		t.Fatalf("failed to send unauthorized request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for wrong token, got %d", resp.StatusCode)
	}
}
