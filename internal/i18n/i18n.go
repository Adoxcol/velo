// Package i18n translates Velo's interface. Pages and Go code are written in
// English; locales/app/<lang>.json maps each English string to its
// translation (keys may hold {0} placeholders, see runtime.js). Missing
// strings stay in English.
package i18n

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed locales/app/*.json
var locales embed.FS

//go:embed runtime.js
var runtimeJS string

// Language is a supported interface language.
type Language struct {
	Code string `json:"code"` // "bn", "pt-BR", ...
	Name string `json:"name"` // in the language itself
}

// Languages lists the interface languages; English first.
var Languages = []Language{
	{"en", "English"},
	{"bn", "বাংলা"},
	{"de", "Deutsch"},
	{"es", "Español"},
	{"fr", "Français"},
	{"hi", "हिन्दी"},
	{"ja", "日本語"},
	{"pt-BR", "Português (Brasil)"},
	{"zh-CN", "简体中文"},
}

var (
	mu    sync.Mutex
	cache = map[string]map[string]string{}
)

// Resolve turns a setting ("" = follow Windows) into a supported code.
func Resolve(setting string) string {
	if setting != "" && supported(setting) {
		return setting
	}
	langs := systemLanguages()
	for _, l := range langs {
		if c := match(l); c != "" {
			return c
		}
	}
	return "en"
}

func supported(code string) bool {
	for _, l := range Languages {
		if l.Code == code {
			return true
		}
	}
	return false
}

// match maps a Windows language tag ("pt-BR", "bn-IN", "zh-Hans-CN") to a
// supported code.
func match(tag string) string {
	tag = strings.ToLower(tag)
	switch {
	case strings.HasPrefix(tag, "pt"):
		return "pt-BR"
	case strings.HasPrefix(tag, "zh-hans"), tag == "zh-cn", tag == "zh-sg", tag == "zh":
		return "zh-CN"
	case strings.HasPrefix(tag, "ja"):
		return "ja"
	}
	base, _, _ := strings.Cut(tag, "-")
	for _, l := range Languages {
		if strings.EqualFold(l.Code, base) {
			return l.Code
		}
	}
	return ""
}

// Dict returns the translations for a language code (empty for English).
func Dict(code string) map[string]string {
	mu.Lock()
	defer mu.Unlock()
	if d, ok := cache[code]; ok {
		return d
	}
	d := map[string]string{}
	if b, err := locales.ReadFile("locales/app/" + code + ".json"); err == nil {
		json.Unmarshal(b, &d)
	}
	cache[code] = d
	return d
}

// T translates one English string.
func T(code, s string) string {
	if v, ok := Dict(code)[s]; ok && v != "" {
		return v
	}
	return s
}

// Script is the JavaScript a window runs before its page: the translator
// and the strings for code.
func Script(code string) string {
	d, _ := json.Marshal(Dict(code))
	return runtimeJS + "\nveloI18n.setup(" + string(d) + ");addEventListener(\"DOMContentLoaded\",()=>{document.documentElement.lang=" + jsonString(code) + "});"
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

// Inject puts the translator for code at the top of a page's <head>, so it
// runs before the page's own scripts. ("</" is escaped so translations that
// contain markup can't end the script tag.)
func Inject(page, code string) string {
	script := strings.ReplaceAll(Script(code), "</", `<\/`)
	return strings.Replace(page, "<head>", "<head><script>"+script+"</script>", 1)
}
