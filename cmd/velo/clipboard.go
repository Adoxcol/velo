package main

import (
	"errors"
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
		toast.Show(a.tr("Download copied link?"), linkName(url)+"\n"+a.tr("Click to download with Velo."), func() {
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

var (
	pEmptyClipboard   = user32n.NewProc("EmptyClipboard")
	pSetClipboardData = user32n.NewProc("SetClipboardData")
	pGlobalAlloc      = kernel32n.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32n.NewProc("GlobalFree")
)

// setClipboardText puts text on the clipboard. Velo's windows can't use the
// browser clipboard API (their pages aren't a secure context), so they call
// this instead.
func setClipboardText(text string) error {
	u, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	var opened uintptr
	for i := 0; i < 10 && opened == 0; i++ { // another program may be holding it for a moment
		if opened, _, _ = pOpenClipboard.Call(0); opened == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if opened == 0 {
		return errors.New("the clipboard is busy")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	const gmemMoveable = 0x0002
	size := uintptr(len(u) * 2)
	h, _, _ := pGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return errors.New("out of memory")
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return errors.New("couldn't lock clipboard memory")
	}
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	copy(unsafe.Slice((*uint16)(ptr), len(u)), u)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		pGlobalFree.Call(h) // still ours if the clipboard didn't take it
		return errors.New("couldn't set the clipboard")
	}
	return nil
}
