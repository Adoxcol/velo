package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// The user's PATH, so veloctl can be run from any terminal. Only the
// current user's PATH is changed; no administrator rights are needed.
const (
	envKey    = `Environment`
	sysEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	// Windows shell32.dll drops the entire User PATH on WM_SETTINGCHANGE when
	// System PATH + ";" + User PATH reaches 4,096 characters.
	maxCombinedPathLen = 4000
)

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

func readSystemPath() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, sysEnvKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("Path")
	return v
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

func expandPercent(s string) string {
	for _, k := range []string{"LOCALAPPDATA", "USERPROFILE", "APPDATA", "ProgramFiles", "ProgramFiles(x86)", "SystemRoot", "windir"} {
		if v := os.Getenv(k); v != "" {
			s = strings.ReplaceAll(s, "%"+k+"%", v)
		}
	}
	return s
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(expandPercent(a)), filepath.Clean(expandPercent(b)))
}

// compactPathParts deduplicates entries, strips entries already in System PATH,
// and shortens %LOCALAPPDATA% / %USERPROFILE% prefixes so combined PATH stays
// under Windows's 4,095-character WM_SETTINGCHANGE limit.
func compactPathParts(parts []string, sysPath string, canExpand bool) []string {
	var sysParts []string
	for _, s := range pathParts(sysPath) {
		sysParts = append(sysParts, s)
	}
	inList := func(list []string, target string) bool {
		for _, item := range list {
			if samePath(item, target) {
				return true
			}
		}
		return false
	}
	la := strings.TrimRight(os.Getenv("LOCALAPPDATA"), `\`)
	up := strings.TrimRight(os.Getenv("USERPROFILE"), `\`)
	var out []string
	for _, s := range parts {
		if inList(out, s) || inList(sysParts, s) {
			continue
		}
		if canExpand {
			exp := expandPercent(s)
			if la != "" && strings.HasPrefix(strings.ToLower(exp), strings.ToLower(la)+`\`) {
				s = `%LOCALAPPDATA%` + exp[len(la):]
			} else if up != "" && strings.HasPrefix(strings.ToLower(exp), strings.ToLower(up)+`\`) {
				s = `%USERPROFILE%` + exp[len(up):]
			}
		}
		out = append(out, s)
	}
	return out
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
	v := strings.Join(parts, ";")
	if sys := readSystemPath(); len(sys)+1+len(expandPercent(v)) >= maxCombinedPathLen {
		parts = compactPathParts(parts, sys, typ != registry.SZ)
		v = strings.Join(parts, ";")
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, envKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
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
