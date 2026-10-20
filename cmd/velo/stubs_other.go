//go:build !windows

package main

import "errors"

type notifier struct{}

func startNotifier() *notifier                             { return &notifier{} }
func (n *notifier) Show(title, msg string, onClick func()) {}
func (n *notifier) Close()                                 {}

var toast *notifier

func setAutostart(on bool) error { return nil }
func autostartEnabled() bool     { return false }
func keepAwake(on bool)          {}
func debugInfo() string          { return "debug info stub" }
func (a *app) debugInfo() string { return "debug info stub" }
func pickFolder(hwnd uintptr, title, start string) (string, error) {
	return "", errors.New("not supported")
}
func pickSaveFile(hwnd uintptr, title, defaultName string) (string, error) {
	return "", errors.New("not supported")
}
func migrateFromIDM()                    {}
func migrateFromMyDM()                   {}
func runTray(a *app, openWindow bool)    {}
func showMain(a *app)                    {}
func evalMain(js string)                 {}
func closeMain()                         {}
func mainVisible() bool                  { return false }
func reloadWindows(a *app)               {}
func (a *app) updateTaskbar()            {}
func clipboardText() (string, bool)      { return "", false }
func setClipboardText(text string) error { return nil }
func showCapture(a *app)                 {}
func (a *app) runMini()                  {}
func hasMiniWindow() bool                { return false }
func closeMini()                         {}
func appIcon() []byte                    { return nil }
func setWindowIcon(hwnd uintptr)         {}

type thumbResult struct {
	Src  string `json:"src"`
	Mode string `json:"mode"`
}

func (a *app) itemPreview(id int) thumbResult       { return thumbResult{} }
func (a *app) fileTypeIcon(name string) thumbResult { return thumbResult{} }
func youtubeThumbURL(rawURL string) string          { return "" }

type videoToolInfo struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type videoToolsState struct {
	Tools      []videoToolInfo `json:"tools"`
	Installing bool            `json:"installing"`
	Step       string          `json:"step,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func (a *app) videoToolsStatus() videoToolsState {
	return videoToolsState{Tools: make([]videoToolInfo, 3)}
}
func (a *app) installVideoTools() error { return nil }
