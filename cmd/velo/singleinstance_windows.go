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
