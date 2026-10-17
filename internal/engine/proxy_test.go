package engine

import (
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
