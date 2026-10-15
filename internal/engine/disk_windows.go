package engine

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// freeSpace returns the bytes free to this user on dir's drive, or -1 if
// it can't tell (then the download just tries).
var freeSpace = func(dir string) int64 {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return -1
	}
	var free uint64
	if r, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0); r == 0 {
		return -1
	}
	return int64(free)
}
