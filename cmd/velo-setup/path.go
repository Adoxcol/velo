package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// The user's PATH, so veloctl can be run from any terminal. Only the
// current user's PATH is changed; no administrator rights are needed.
const envKey = `Environment`

func readUserPath() (string, uint32) {
	k, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.QUERY_VALUE)
	if err != nil {
		return "", registry.EXPAND_SZ
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if err != nil {
		return "", registry.EXPAND_SZ
	}
	return v, typ
}

func pathParts(p string) []string {
	var out []string
	for _, s := range strings.Split(p, ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// onPath reports whether dir is already in the user's PATH.
func onPath(dir string) bool {
	p, _ := readUserPath()
	for _, s := range pathParts(p) {
		if samePath(s, dir) {
			return true
		}
	}
	return false
}

// setOnPath adds dir to the user's PATH (add) or removes it (!add).
func setOnPath(dir string, add bool) error {
	p, typ := readUserPath()
	var parts []string
	for _, s := range pathParts(p) {
		if !samePath(s, dir) {
			parts = append(parts, s)
		}
	}
	if add {
		parts = append(parts, dir)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, envKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	v := strings.Join(parts, ";")
	if typ == registry.SZ {
		err = k.SetStringValue("Path", v)
	} else {
		err = k.SetExpandStringValue("Path", v)
	}
	if err != nil {
		return err
	}
	broadcastEnvChange()
	return nil
}

// broadcastEnvChange tells Explorer the environment changed, so terminals
// opened from now on see the new PATH.
func broadcastEnvChange() {
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	env, _ := syscall.UTF16PtrFromString("Environment")
	var result uintptr
	syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 2000, uintptr(unsafe.Pointer(&result)))
}
