package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
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

func TestAddInvalidScheme(t *testing.T) {
	a := &app{token: "test-token"}
	req := httptest.NewRequest("POST", "/add", strings.NewReader(`{"url":"file:///etc/passwd"}`))
	req.Header.Set(ipc.TokenHeader, a.token)
	res := httptest.NewRecorder()
	a.handle(res, req)
	if !strings.Contains(res.Body.String(), "invalid URL scheme") {
		t.Fatalf("expected error for non-http URL, got: %s", res.Body.String())
	}
}

func TestAddBatchInvalidScheme(t *testing.T) {
	a := &app{token: "test-token"}
	b := batchReq{
		Links: []struct {
			URL  string `json:"url"`
			Text string `json:"text"`
		}{
			{URL: "httpfoo://example.com/file.zip", Text: "bad1"},
			{URL: "file:///etc/passwd", Text: "bad2"},
			{URL: "https://example.com/file.zip", Text: "good"},
		},
	}
	urls := []string{
		"httpfoo://example.com/file.zip",
		"file:///etc/passwd",
		"https://example.com/file.zip",
	}

	addedStart := a.addBatch(b, urls, true)
	if addedStart != 1 {
		t.Fatalf("expected 1 valid URL added when start=true, got %d", addedStart)
	}

	addedPaused := a.addBatch(b, urls, false)
	if addedPaused != 1 {
		t.Fatalf("expected 1 valid URL added when start=false, got %d", addedPaused)
	}
}

func TestHandleBatchFiltersInvalidSchemes(t *testing.T) {
	a := &app{token: "test-token"}

	// Test 1: All links invalid -> expected "no links" error
	reqAllInvalid := httptest.NewRequest("POST", "/batch", strings.NewReader(`{
		"page": "http://example.com",
		"links": [
			{"url": "file:///etc/passwd", "text": "bad1"},
			{"url": "javascript:alert(1)", "text": "bad2"}
		]
	}`))
	reqAllInvalid.Header.Set(ipc.TokenHeader, a.token)
	resAllInvalid := httptest.NewRecorder()
	a.handle(resAllInvalid, reqAllInvalid)
	if !strings.Contains(resAllInvalid.Body.String(), "no links") {
		t.Fatalf("expected 'no links' error for batch with only invalid schemes, got: %s", resAllInvalid.Body.String())
	}

	// Test 2: Mixed links -> expected valid link retained, invalid filtered out
	reqMixed := httptest.NewRequest("POST", "/batch", strings.NewReader(`{
		"page": "http://example.com",
		"links": [
			{"url": "file:///etc/passwd", "text": "bad1"},
			{"url": "https://example.com/file.zip", "text": "good"}
		]
	}`))
	reqMixed.Header.Set(ipc.TokenHeader, a.token)
	resMixed := httptest.NewRecorder()
	a.handle(resMixed, reqMixed)
	if !strings.Contains(resMixed.Body.String(), `"ok":true`) || !strings.Contains(resMixed.Body.String(), `"id":1`) {
		t.Fatalf("expected ok with count 1 for mixed batch, got: %s", resMixed.Body.String())
	}
	if a.batch == nil || len(a.batch.Links) != 1 || a.batch.Links[0].URL != "https://example.com/file.zip" {
		t.Fatalf("expected batch to contain only valid URL, got: %+v", a.batch)
	}
}

func BenchmarkAddBatch(b *testing.B) {
	for _, n := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("items-%d", n), func(b *testing.B) {
			a := &app{eng: engine.New(b.TempDir()), home: b.TempDir()}
			req := batchReq{
				Page:    "https://example.com/gallery/list",
				Cookies: "session=12345",
			}
			var urls []string
			for i := 1; i <= n; i++ {
				u := "https://example.com/files/download" + fmt.Sprint(i) + ".zip"
				req.Links = append(req.Links, struct {
					URL  string `json:"url"`
					Text string `json:"text"`
				}{URL: u, Text: "File " + fmt.Sprint(i)})
				urls = append(urls, u)
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = a.addBatch(req, urls, false)
			}
		})
	}
}
