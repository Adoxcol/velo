package i18n

import (
	"strings"
	"testing"
)

func TestLanguagesLoad(t *testing.T) {
	for _, l := range Languages[1:] {
		d := Dict(l.Code)
		if len(d) == 0 {
			t.Errorf("%s (%s): no translations", l.Code, l.Name)
			continue
		}
		if d["Settings"] == "" || d["Settings"] == "Settings" && l.Code != "de" {
			t.Errorf("%s: 'Settings' isn't translated", l.Code)
		}
	}
	if len(Dict("en")) != 0 {
		t.Error("English should have no translations")
	}
}

func TestResolveAndMatch(t *testing.T) {
	for in, want := range map[string]string{"pt-PT": "pt-BR", "pt-BR": "pt-BR", "zh-Hans-CN": "zh-CN", "bn-IN": "bn", "de-AT": "de", "ja-JP": "ja", "ko-KR": "", "en-GB": "en"} {
		if got := match(in); got != want {
			t.Errorf("match(%q) = %q, want %q", in, got, want)
		}
	}
	if Resolve("bn") != "bn" || Resolve("xx") == "xx" {
		t.Error("Resolve should keep supported codes and reject others")
	}
	if T("bn", "Settings") == "Settings" || T("bn", "no such string") != "no such string" {
		t.Error("T")
	}
}

func TestInject(t *testing.T) {
	out := Inject("<html><head><title>x</title></head></html>", "bn")
	if !strings.Contains(out, "veloI18n.setup(") || strings.Count(out, "</script>") != 1 {
		t.Fatalf("translator not injected safely: %.200s", out)
	}
}
