//go:build windows

package i18n

import "golang.org/x/sys/windows"

func systemLanguages() []string {
	langs, _ := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	return langs
}
