//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	shell32dll                  = syscall.NewLazyDLL("shell32.dll")
	ole32dll                    = syscall.NewLazyDLL("ole32.dll")
	pShellExecuteW              = shell32dll.NewProc("ShellExecuteW")
	pSHOpenFolderAndSelectItems = shell32dll.NewProc("SHOpenFolderAndSelectItems")
	pILCreateFromPathW          = shell32dll.NewProc("ILCreateFromPathW")
	pILFree                     = shell32dll.NewProc("ILFree")
	pCoInitializeEx             = ole32dll.NewProc("CoInitializeEx")
	pCoUninitialize             = ole32dll.NewProc("CoUninitialize")
)

// openInFolder opens the parent folder in Windows File Explorer and selects the file.
// It uses the native SHOpenFolderAndSelectItems Shell API for near-instant execution
// without spawning an external explorer.exe launcher process.
func openInFolder(targetPath string) error {
	// COM initialization belongs to the OS thread: keep this goroutine on one
	// thread so CoUninitialize runs where CoInitializeEx did.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	clean := filepath.Clean(targetPath)
	p16, err := syscall.UTF16PtrFromString(clean)
	if err != nil {
		return exec.Command("explorer", "/select,", clean).Start()
	}

	// Initialize COM apartment for the calling goroutine if needed.
	// COINIT_APARTMENTTHREADED (0x2) | COINIT_DISABLE_OLE1DDE (0x4)
	hr, _, _ := pCoInitializeEx.Call(0, 0x6)
	if hr == 0 || hr == 1 { // S_OK or S_FALSE (already initialized)
		defer pCoUninitialize.Call()
	}

	pidl, _, _ := pILCreateFromPathW.Call(uintptr(unsafe.Pointer(p16)))
	if pidl == 0 {
		return exec.Command("explorer", "/select,", clean).Start()
	}
	defer pILFree.Call(pidl)

	// If cidl is 0 and apidl is nil, pidl specifies the single item to be selected
	// and its parent folder is automatically opened.
	res, _, _ := pSHOpenFolderAndSelectItems.Call(pidl, 0, 0, 0)
	if int32(res) < 0 { // FAILED(hr)
		return exec.Command("explorer", "/select,", clean).Start()
	}
	return nil
}

// openPath opens a file with its default associated application or opens a directory
// in File Explorer using the native ShellExecuteW API.
func openPath(targetPath string) error {
	clean := filepath.Clean(targetPath)
	p16, err := syscall.UTF16PtrFromString(clean)
	if err != nil {
		return exec.Command("explorer", clean).Start()
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	const swNormal = 1
	res, _, _ := pShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(p16)),
		0,
		0,
		swNormal,
	)
	// ShellExecute returns an HINSTANCE value > 32 on success.
	if res <= 32 {
		return exec.Command("explorer", clean).Start()
	}
	return nil
}
