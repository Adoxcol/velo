package engine

import (
	"net/http"
	"reflect"
	"testing"
)

func TestParseProxy(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			want:    "",
			wantErr: false,
		},
		{
			name:    "system mode exact",
			input:   "system",
			want:    "",
			wantErr: false,
		},
		{
			name:    "system mode mixed case and padding",
			input:   "  SysTem  ",
			want:    "",
			wantErr: false,
		},
		{
			name:    "none mode exact",
			input:   "none",
			want:    "none",
			wantErr: false,
		},
		{
			name:    "none mode uppercase with spaces",
			input:   " NONE ",
			want:    "none",
			wantErr: false,
		},
		{
			name:    "host and port without scheme (defaults to http)",
			input:   "127.0.0.1:8080",
			want:    "http://127.0.0.1:8080",
			wantErr: false,
		},
		{
			name:    "hostname and port without scheme",
			input:   "proxy.example.com:3128",
			want:    "http://proxy.example.com:3128",
			wantErr: false,
		},
		{
			name:    "http URL",
			input:   "http://127.0.0.1:8080",
			want:    "http://127.0.0.1:8080",
			wantErr: false,
		},
		{
			name:    "https URL",
			input:   "https://proxy.example.com:8443",
			want:    "https://proxy.example.com:8443",
			wantErr: false,
		},
		{
			name:    "socks5 URL",
			input:   "socks5://127.0.0.1:1080",
			want:    "socks5://127.0.0.1:1080",
			wantErr: false,
		},
		{
			name:    "socks5h URL",
			input:   "socks5h://127.0.0.1:1080",
			want:    "socks5h://127.0.0.1:1080",
			wantErr: false,
		},
		{
			name:    "URL with credentials",
			input:   "http://user:pass@proxy.example.com:8080",
			want:    "http://user:pass@proxy.example.com:8080",
			wantErr: false,
		},
		{
			name:    "unsupported scheme (ftp)",
			input:   "ftp://127.0.0.1:21",
			want:    "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme (socks4)",
			input:   "socks4://127.0.0.1:1080",
			want:    "",
			wantErr: true,
		},
		{
			name:    "missing port",
			input:   "127.0.0.1",
			want:    "",
			wantErr: true,
		},
		{
			name:    "http URL missing port",
			input:   "http://nohost",
			want:    "",
			wantErr: true,
		},
		{
			name:    "http URL trailing colon without port",
			input:   "http://host:",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			input:   "::",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProxy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProxy(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseProxy(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseProxyServer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "empty",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "single host:port",
			input: "127.0.0.1:8080",
			want:  map[string]string{"": "127.0.0.1:8080"},
		},
		{
			name:  "multiple schemes",
			input: "http=127.0.0.1:8080;HTTPS=127.0.0.1:8081;socks=127.0.0.1:1080",
			want: map[string]string{
				"http":  "127.0.0.1:8080",
				"https": "127.0.0.1:8081",
				"socks": "127.0.0.1:1080",
			},
		},
		{
			name:  "socks only fallback",
			input: "socks=127.0.0.1:1080",
			want: map[string]string{
				"socks": "127.0.0.1:1080",
				"":      "127.0.0.1:1080",
			},
		},
		{
			name:  "trimmed whitespace and empty parts",
			input: " ; http=127.0.0.1:8080 ; ; ",
			want: map[string]string{
				"http": "127.0.0.1:8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProxyServer(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseProxyServer(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"*.corp.com", "sub.corp.com", true},
		{"*.corp.com", "corp.com", false},
		{"10.*", "10.0.0.1", true},
		{"10.*", "192.168.0.1", false},
		{"*a*b*", "1a2b3", true},
		{"*a*b*", "1a2", false},
		{"foo*bar", "foobar", true},
		{"foo*bar", "foo123bar", true},
		{"foo*bar", "foobaz", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.host, func(t *testing.T) {
			got := wildcardMatch(tt.pattern, tt.host)
			if got != tt.want {
				t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}

func BenchmarkWildcardMatch(b *testing.B) {
	benchmarks := []struct {
		pattern string
		host    string
	}{
		{"example.com", "example.com"},
		{"*.corp.com", "sub.corp.com"},
		{"10.*", "10.0.0.1"},
		{"*a*b*", "1a2b3"},
		{"foo*bar", "foo123bar"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, bm := range benchmarks {
			wildcardMatch(bm.pattern, bm.host)
		}
	}
}

func TestEngineProxy(t *testing.T) {
	e := New(t.TempDir())

	req, err := http.NewRequest("GET", "http://example.com/file.zip", nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("none mode", func(t *testing.T) {
		e.SetProxy("none")
		u, err := e.proxyFor(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u != nil {
			t.Errorf("expected nil proxy for 'none', got %v", u)
		}
	})

	t.Run("explicit proxy URL", func(t *testing.T) {
		proxyURL := "http://127.0.0.1:8080"
		e.SetProxy(proxyURL)
		u, err := e.proxyFor(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u == nil || u.String() != proxyURL {
			t.Errorf("proxyFor = %v, want %s", u, proxyURL)
		}
	})

	t.Run("system mode", func(t *testing.T) {
		e.SetProxy("system")
		_, err := e.proxyFor(req)
		if err != nil {
			t.Fatalf("unexpected error in system mode: %v", err)
		}
	})

	t.Run("empty string mode", func(t *testing.T) {
		e.SetProxy("")
		_, err := e.proxyFor(req)
		if err != nil {
			t.Fatalf("unexpected error in empty mode: %v", err)
		}
	})
}
