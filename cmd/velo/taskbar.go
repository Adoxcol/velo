package main

import (
	"log"
	"syscall"
	"unsafe"
)

// The taskbar button fills with download progress, like a file copy.

var (
	clsidTaskbarList = syscall.GUID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11D0, Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidITaskbarList3 = syscall.GUID{Data1: 0xEA1AFB91, Data2: 0x9E28, Data3: 0x4B86, Data4: [8]byte{0x90, 0xE9, 0x9E, 0x9F, 0x8A, 0x5E, 0xEF, 0xAF}}
)

const (
	vtHrInit           = 3
	vtSetProgressValue = 9
	vtSetProgressState = 10

	tbpfNoProgress    = 0
	tbpfIndeterminate = 1
	tbpfNormal        = 2
)

// Owned by the main window's thread: only touched inside win.Dispatch.
var taskbar struct {
	obj   unsafe.Pointer
	hwnd  uintptr
	state uintptr
}

// taskbarProgress works out overall progress: done and total bytes of the
// running downloads; total is 0 when a running download's size is unknown.
func (a *app) taskbarProgress() (running bool, done, total int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, it := range a.items {
		if !it.running {
			continue
		}
		running = true
		_, t := it.d.Info()
		if t <= 0 || it.d.Req.Kind != "" { // streams only know a percentage
			continue
		}
		done += it.d.Done.Load()
		total += t
	}
	return
}

// updateTaskbar shows the progress on the main window's taskbar button.
func (a *app) updateTaskbar() {
	running, done, total := a.taskbarProgress()
	winMu.Lock()
	w := win
	winMu.Unlock()
	if w == nil {
		return
	}
	w.Dispatch(func() {
		hwnd := uintptr(w.Window())
		if taskbar.obj == nil || taskbar.hwnd != hwnd {
			if taskbar.obj != nil {
				comCall(taskbar.obj, vtRelease)
				taskbar.obj = nil
			}
			coInitializeEx.Call(0, coinitApartmentThrd)
			var obj unsafe.Pointer
			if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
				uintptr(unsafe.Pointer(&iidITaskbarList3)), uintptr(unsafe.Pointer(&obj))); failed(hr) {
				log.Printf("taskbar progress unavailable: %#x", hr)
				return
			}
			if hr := comCall(obj, vtHrInit); failed(hr) {
				log.Printf("taskbar progress unavailable: HrInit %#x", hr)
				comCall(obj, vtRelease)
				return
			}
			taskbar.obj, taskbar.hwnd, taskbar.state = obj, hwnd, tbpfNoProgress
		}
		state := uintptr(tbpfNoProgress)
		switch {
		case running && total > 0:
			state = tbpfNormal
		case running:
			state = tbpfIndeterminate
		}
		if state != taskbar.state {
			if hr := comCall(taskbar.obj, vtSetProgressState, hwnd, state); failed(hr) {
				log.Printf("taskbar progress: %#x", hr)
			}
			taskbar.state = state
		}
		if state == tbpfNormal {
			comCall(taskbar.obj, vtSetProgressValue, hwnd, uintptr(done), uintptr(total))
		}
	})
}
