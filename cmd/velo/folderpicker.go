//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pickFolder shows the standard Windows folder picker (IFileOpenDialog with
// FOS_PICKFOLDERS) owned by hwnd, starting in start. It returns "" if the
// user cancels. Must be called on a thread with a message loop (the window's).

var (
	ole32                 = syscall.NewLazyDLL("ole32.dll")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	coInitializeEx        = ole32.NewProc("CoInitializeEx")
	coCreateInstance      = ole32.NewProc("CoCreateInstance")
	coTaskMemFree         = ole32.NewProc("CoTaskMemFree")
	shCreateItemFromParse = shell32.NewProc("SHCreateItemFromParsingName")

	clsidFileOpenDialog = syscall.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = syscall.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = syscall.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

const (
	fosPickFolders      = 0x20
	fosForceFileSystem  = 0x40
	fosPathMustExist    = 0x800
	sigdnFileSysPath    = 0x80058000
	errCancelled        = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)
	clsctxInprocServer  = 1
	coinitApartmentThrd = 2

	// vtable slots
	vtRelease    = 2
	vtShow       = 3
	vtSetOptions = 9
	vtGetOptions = 10
	vtSetFolder  = 12
	vtSetTitle   = 17
	vtGetResult  = 20
	vtGetDisplay = 5 // IShellItem::GetDisplayName
)

// comObject is the layout of any COM interface pointer: a vtable pointer.
type comObject struct{ vtbl *[32]uintptr }

// comCall calls method slot on a COM object.
func comCall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	fn := (*comObject)(obj).vtbl[slot]
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

func pickFolder(hwnd uintptr, title, start string) (string, error) {
	coInitializeEx.Call(0, coinitApartmentThrd) // fine if already initialised

	var dlg unsafe.Pointer
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg))); failed(hr) {
		return "", errors.New("can't open the folder picker")
	}
	defer comCall(dlg, vtRelease)

	var opts uintptr
	comCall(dlg, vtGetOptions, uintptr(unsafe.Pointer(&opts)))
	comCall(dlg, vtSetOptions, opts|fosPickFolders|fosForceFileSystem|fosPathMustExist)
	if t, err := syscall.UTF16PtrFromString(title); err == nil {
		comCall(dlg, vtSetTitle, uintptr(unsafe.Pointer(t)))
	}
	if start != "" {
		if p, err := syscall.UTF16PtrFromString(start); err == nil {
			var item unsafe.Pointer
			if hr, _, _ := shCreateItemFromParse.Call(uintptr(unsafe.Pointer(p)), 0,
				uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); !failed(hr) {
				comCall(dlg, vtSetFolder, uintptr(item))
				comCall(item, vtRelease)
			}
		}
	}

	hr := comCall(dlg, vtShow, hwnd)
	if uint32(hr) == errCancelled {
		return "", nil
	}
	if failed(hr) {
		return "", errors.New("folder picker failed")
	}
	var item unsafe.Pointer
	if hr := comCall(dlg, vtGetResult, uintptr(unsafe.Pointer(&item))); failed(hr) {
		return "", errors.New("no folder selected")
	}
	defer comCall(item, vtRelease)
	var name *uint16
	if hr := comCall(item, vtGetDisplay, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); failed(hr) {
		return "", errors.New("that location isn't a folder on disk")
	}
	defer coTaskMemFree.Call(uintptr(unsafe.Pointer(name)))
	return windows.UTF16PtrToString(name), nil
}
