//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func singleInstance() bool {
	name, _ := syscall.UTF16PtrFromString(`Local\Velo.SingleInstance`)
	h, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").
		Call(0, 0, uintptr(unsafe.Pointer(name)))
	return h != 0 && err != syscall.Errno(183) // ERROR_ALREADY_EXISTS
}

// allowAnyForeground lets the running Velo bring its window in front when
// it is started again. Windows only lets the foreground app (this new
// process, which the user just launched) grant that.
func allowAnyForeground() {
	syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(^uintptr(0)) // ASFW_ANY
}
