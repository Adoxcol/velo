// Package ipc is the link between the native messaging host and the app.
//
// The app listens on a random 127.0.0.1 port and writes {port, token} to
// %APPDATA%\Velo\endpoint.json. Every request must carry the token, so web
// pages that probe localhost can't add downloads.
package ipc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Job struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Referrer  string `json:"referrer"`
	Filename  string `json:"filename"`
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
	Kind      string `json:"kind"`  // "" = file, "hls" = video stream
	Title     string `json:"title"` // page title, used to name videos
}

type Reply struct {
	OK    bool   `json:"ok"`
	ID    int    `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

type Endpoint struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

const TokenHeader = "X-Velo-Token"

// Dir is Velo's data folder: %APPDATA%\velo, or $VELO_HOME if set (tests).
func Dir() string {
	if d := os.Getenv("VELO_HOME"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "Velo")
}

func endpointPath() string { return filepath.Join(Dir(), "endpoint.json") }

func NewToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func WriteEndpoint(e Endpoint) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(e)
	tmp := endpointPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, endpointPath())
}

func ReadEndpoint() (Endpoint, error) {
	var e Endpoint
	b, err := os.ReadFile(endpointPath())
	if err != nil {
		return e, err
	}
	return e, json.Unmarshal(b, &e)
}

func RemoveEndpoint() { os.Remove(endpointPath()) }

// Call sends a command (add, list, pause, resume, show) to the running app
// and returns the raw JSON response.
func Call(cmd, id string, body []byte) ([]byte, error) {
	e, err := ReadEndpoint()
	if err != nil {
		return nil, errors.New("Velo is not running")
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/%s?id=%s", e.Port, cmd, id), bytes.NewReader(body))
	req.Header.Set(TokenHeader, e.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, errors.New("Velo is not running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Velo: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}
