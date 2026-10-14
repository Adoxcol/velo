package main

import (
	"log"
	"path"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"velo/internal/engine"
)

// Clipboard monitor: when a link to a downloadable file is
// copied anywhere, offer to download it with a notification. Nothing is
// downloaded unless the notification is clicked.

var (
	pGetClipboardSeq   = user32n.NewProc("GetClipboardSequenceNumber")
	pOpenClipboard     = user32n.NewProc("OpenClipboard")
	pCloseClipboard    = user32n.NewProc("CloseClipboard")
	pGetClipboardData  = user32n.NewProc("GetClipboardData")
	pIsClipboardFormat = user32n.NewProc("IsClipboardFormatAvailable")
	pGlobalLock        = kernel32n.NewProc("GlobalLock")
	pGlobalUnlock      = kernel32n.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

func clipboardText() (string, bool) {
	if r, _, _ := pIsClipboardFormat.Call(cfUnicodeText); r == 0 {
		return "", false
	}
	if r, _, _ := pOpenClipboard.Call(0); r == 0 {
		return "", false // busy; try again on the next change
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", false
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return "", false
	}
	defer pGlobalUnlock.Call(h)
	// Read at most 4096 UTF-16 units: a link is short, anything longer isn't one.
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p)) // memory owned by the clipboard, valid while locked
	buf := unsafe.Slice((*uint16)(ptr), 4096)
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	if n == len(buf) {
		return "", false
	}
	return syscall.UTF16ToString(buf[:n]), true
}

// copiedLink returns the URL in text if it's a single downloadable link.
func copiedLink(text string) (url, kind string, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 2048 || strings.ContainsAny(text, " \t\r\n") {
		return "", "", false
	}
	if ok, kind := engine.Downloadable(text); ok {
		return text, kind, true
	}
	return "", "", false
}

func linkName(u string) string {
	name := path.Base(strings.SplitN(strings.SplitN(u, "?", 2)[0], "#", 2)[0])
	if name == "" || name == "/" || name == "." {
		return u
	}
	return name
}

// watchClipboard polls the clipboard's change counter (cheap) and only reads
// the text when something new was copied. Runs until the app exits.
func (a *app) watchClipboard() {
	last, _, _ := pGetClipboardSeq.Call() // ignore whatever was copied before start
	offered := ""
	for range time.Tick(500 * time.Millisecond) {
		seq, _, _ := pGetClipboardSeq.Call()
		if seq == last {
			continue
		}
		last = seq
		if !a.getSettings().WatchClipboard {
			continue
		}
		text, ok := clipboardText()
		if !ok {
			continue
		}
		url, kind, ok := copiedLink(text)
		if !ok || url == offered || a.hasURL(url) {
			continue
		}
		offered = url
		log.Printf("clipboard: offering %s", url)
		toast.Show("Download copied link?", linkName(url)+"\nClick to download with Velo.", func() {
			a.add(engine.Request{URL: url, Kind: kind})
			a.show()
		})
	}
}

// hasURL reports whether the URL is already in the download list.
func (a *app) hasURL(u string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, it := range a.items {
		if it.d.Req.URL == u {
			return true
		}
	}
	return false
}
