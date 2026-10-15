// velo-host is the Chrome/Edge/Firefox native messaging host. The browser starts it per message; it
// reads one length-prefixed JSON message from stdin, forwards it to the
// app (starting the app if needed) and writes the reply to stdout.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"velo/internal/ipc"
)

func readMsg(r io.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	if n > 1<<20 {
		return nil, errors.New("message too large")
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func writeMsg(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	binary.Write(w, binary.LittleEndian, uint32(len(b)))
	w.Write(b)
}

// paths maps the extension's message types to the app's endpoints.
var paths = map[string]string{"download": "/add", "ping": "/ping", "show": "/show", "batch": "/batch"}

func post(e ipc.Endpoint, path string, body []byte) (ipc.Reply, error) {
	var r ipc.Reply
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d%s", e.Port, path), bytes.NewReader(body))
	req.Header.Set(ipc.TokenHeader, e.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	return r, json.NewDecoder(resp.Body).Decode(&r)
}

// startApp launches velo.exe from the host's own directory and waits for
// it to publish its endpoint.
func startApp() (ipc.Endpoint, error) {
	self, _ := os.Executable()
	app := filepath.Join(filepath.Dir(self), "velo.exe")
	cmd := exec.Command(app, "-background")
	if err := cmd.Start(); err != nil {
		return ipc.Endpoint{}, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if e, err := ipc.ReadEndpoint(); err == nil && e.PID == pid {
			return e, nil
		}
	}
	return ipc.Endpoint{}, errors.New("app did not start")
}

func main() {
	msg, err := readMsg(os.Stdin)
	if err != nil {
		writeMsg(os.Stdout, ipc.Reply{Error: err.Error()})
		return
	}
	var head struct {
		Type string `json:"type"`
	}
	json.Unmarshal(msg, &head)
	path, ok := paths[head.Type]
	if !ok {
		writeMsg(os.Stdout, ipc.Reply{Error: "unknown message type " + head.Type})
		return
	}
	e, err := ipc.ReadEndpoint()
	var r ipc.Reply
	if err == nil {
		allowForeground(e.PID)
		r, err = post(e, path, msg)
	}
	if err != nil { // stale endpoint or app not running
		if e, err = startApp(); err == nil {
			allowForeground(e.PID)
			r, err = post(e, path, msg)
		}
	}
	if err != nil {
		r = ipc.Reply{Error: err.Error()}
	}
	writeMsg(os.Stdout, r)
}

// allowForeground lets Velo bring its download dialog in front of the
// browser. Windows only lets the foreground app (the browser, which started
// this host) grant that.
func allowForeground(pid int) {
	syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(uintptr(pid))
}
