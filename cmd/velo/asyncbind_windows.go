//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"
)

// WebView bindings run on the window thread. Dispatch only the result back
// there; file access, shell extensions and child processes may block.
func bindWindowAsync[A, R any](w webview2.WebView, closed *atomic.Bool, name string, fn func(A) (R, error)) {
	request, result := name+"Request", name+"Result"
	w.Bind(request, func(id int, arg A) {
		go func() {
			// Shell icon providers may use COM. Workers need their own
			// apartment, initialized and released on the same OS thread.
			runtime.LockOSThread()
			// No UnlockOSThread: the thread exits with the goroutine, taking any
			// hidden windows (shell, COM, WebView2) with it instead of leaving them
			// on a pooled thread that never reads messages and stalls input.
			hr, _, _ := pCoInitializeEx.Call(0, 0x6)
			if hr == 0 || hr == 1 {
				defer pCoUninitialize.Call()
			}
			value, err := fn(arg)
			if closed.Load() {
				return
			}
			data, encodeErr := json.Marshal(value)
			if err == nil {
				err = encodeErr
			}
			failure := "null"
			if err != nil {
				b, _ := json.Marshal(err.Error())
				failure, data = string(b), []byte("null")
			}
			w.Dispatch(func() {
				if !closed.Load() {
					w.Eval(fmt.Sprintf("window.%s(%d,%s,%s)", result, id, data, failure))
				}
			})
		}()
	})
	w.Init(fmt.Sprintf(`(() => {
  let next = 0;
  const pending = new Map();
  window.%s = (id, value, error) => {
    const p = pending.get(id);
    if (!p) return;
    pending.delete(id);
    if (error !== null) p.reject(new Error(error)); else p.resolve(value);
  };
  window.%s = (...args) => new Promise((resolve, reject) => {
    const id = ++next;
    pending.set(id, {resolve, reject});
    const arg = args.length === 0 ? null : (args.length === 1 ? args[0] : args);
    window.%s(id, arg === undefined ? null : arg).catch(error => {
      pending.delete(id); reject(error);
    });
  });
})();`, result, name, request))
}
