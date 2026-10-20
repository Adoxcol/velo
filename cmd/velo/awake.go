//go:build windows

package main

import (
	"log"
	"runtime"
	"syscall"
)

var setThreadExecutionState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var awake = make(chan bool, 1)

// keepAwake stops Windows from sleeping while on is true. The screen can
// still turn off. The request belongs to a thread, so one locked thread
// owns it.
func keepAwake(on bool) {
	select {
	case awake <- on:
	default: // a change is already waiting; the next tick sends again
	}
}

func init() {
	go func() {
		runtime.LockOSThread()
		cur := false
		for on := range awake {
			if on == cur {
				continue
			}
			cur = on
			flags := uintptr(esContinuous)
			if on {
				flags |= esSystemRequired
			}
			if r, _, err := setThreadExecutionState.Call(flags); r == 0 {
				log.Printf("keep awake %v: %v", on, err)
			} else if on {
				log.Print("downloading: keeping Windows awake")
			} else {
				log.Print("idle: Windows may sleep again")
			}
		}
	}()
}
