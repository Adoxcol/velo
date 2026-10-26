//go:build windows

package main

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	webview2 "github.com/jchv/go-webview2"
)

type asyncTestWindow struct {
	webview2.WebView
	binding interface{}
	queue   chan func()
	scripts []string
}

func (w *asyncTestWindow) Bind(_ string, fn interface{}) error { w.binding = fn; return nil }
func (w *asyncTestWindow) Init(string)                         {}
func (w *asyncTestWindow) Dispatch(fn func())                  { w.queue <- fn }
func (w *asyncTestWindow) Eval(js string)                      { w.scripts = append(w.scripts, js) }

func TestWindowAsyncDoesNotBlockUI(t *testing.T) {
	w := &asyncTestWindow{queue: make(chan func(), 1)}
	var closed atomic.Bool
	started, unblock := make(chan struct{}), make(chan struct{})
	defer close(unblock)
	bindWindowAsync(w, &closed, "preview", func(id int) (string, error) {
		close(started)
		<-unblock
		return "image", nil
	})
	returned := make(chan struct{})
	go func() { w.binding.(func(int, int))(1, 41); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("UI callback waited for preview work")
	}
	<-started
	select {
	case <-w.queue:
		t.Fatal("result dispatched before work completed")
	default:
	}
}

func TestWindowAsyncResultAndClosedWindow(t *testing.T) {
	for _, closeBeforeDispatch := range []bool{false, true} {
		w := &asyncTestWindow{queue: make(chan func(), 1)}
		var closed atomic.Bool
		bindWindowAsync(w, &closed, "preview", func(id int) (string, error) { return "image", nil })
		w.binding.(func(int, int))(7, 41)
		select {
		case fn := <-w.queue:
			closed.Store(closeBeforeDispatch)
			fn()
		case <-time.After(time.Second):
			t.Fatal("no result dispatched")
		}
		if closeBeforeDispatch {
			if len(w.scripts) != 0 {
				t.Fatal("evaluated JavaScript on closed window")
			}
		} else if len(w.scripts) != 1 || !strings.Contains(w.scripts[0], `previewResult(7,"image",null)`) {
			t.Fatalf("incorrect RPC result: %v", w.scripts)
		}
	}
}
