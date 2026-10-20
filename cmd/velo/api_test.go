package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/ipc"
)

func TestVideoCaptureOpensRenameDialog(t *testing.T) {
	shown := make(chan struct{}, 1)
	a := &app{
		token:       "test-token",
		settings:    Settings{AskOnCapture: true},
		showPending: func() { shown <- struct{}{} },
	}
	req := httptest.NewRequest("POST", "/add", strings.NewReader(`{"url":"https://www.youtube.com/watch?v=test","kind":"page","format":"mp4","filename":"Actual video.mp4","source":"capture"}`))
	req.Header.Set(ipc.TokenHeader, a.token)
	res := httptest.NewRecorder()
	a.handle(res, req)
	select {
	case <-shown:
	case <-time.After(time.Second):
		t.Fatal("video capture did not open rename dialog")
	}
	p := a.nextPendingDL()
	if p == nil || p.Filename != "Actual video.mp4" || p.Format != "mp4" || len(a.items) != 0 {
		t.Fatalf("video must wait for confirmation with its name and format: %+v", p)
	}
}

func TestAPIAuthorization(t *testing.T) {
	validToken := "secret-token-12345"
	a := &app{token: validToken}

	ts := httptest.NewServer(http.HandlerFunc(a.handle))
	defer ts.Close()

	t.Run("Valid Token", func(t *testing.T) {
		req, err := http.NewRequest("POST", ts.URL+"/ping", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set(ipc.TokenHeader, validToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for valid token, got %d", resp.StatusCode)
		}
	})

	t.Run("Wrong Token", func(t *testing.T) {
		req, err := http.NewRequest("POST", ts.URL+"/ping", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set(ipc.TokenHeader, "wrong-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for wrong token, got %d", resp.StatusCode)
		}
	})

	t.Run("Empty Header Token", func(t *testing.T) {
		req, err := http.NewRequest("POST", ts.URL+"/ping", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set(ipc.TokenHeader, "")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for empty token header, got %d", resp.StatusCode)
		}
	})

	t.Run("Empty App Token", func(t *testing.T) {
		emptyApp := &app{token: ""}
		emptyTs := httptest.NewServer(http.HandlerFunc(emptyApp.handle))
		defer emptyTs.Close()

		req, err := http.NewRequest("POST", emptyTs.URL+"/ping", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set(ipc.TokenHeader, "")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden when app token is empty, got %d", resp.StatusCode)
		}
	})

	t.Run("Non-POST Method", func(t *testing.T) {
		req, err := http.NewRequest("GET", ts.URL+"/ping", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set(ipc.TokenHeader, validToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for GET request, got %d", resp.StatusCode)
		}
	})
}
