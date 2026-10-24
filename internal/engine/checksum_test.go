package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestParseChecksum(t *testing.T) {
	md5Lower := "d41d8cd98f00b204e9800998ecf8427e"
	sha1Lower := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	sha256Lower := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sha512Lower := "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid cases - MD5 (32 hex chars)
		{name: "MD5 lowercase", input: md5Lower, want: md5Lower},
		{name: "MD5 uppercase", input: strings.ToUpper(md5Lower), want: md5Lower},
		{name: "MD5 with prefix", input: "md5:" + md5Lower, want: md5Lower},
		{name: "MD5 with uppercase prefix and hash", input: "MD5:" + strings.ToUpper(md5Lower), want: md5Lower},
		{name: "MD5 with filename", input: md5Lower + "  sample.txt", want: md5Lower},
		{name: "MD5 with binary mode star and filename", input: md5Lower + " *sample.bin", want: md5Lower},

		// Valid cases - SHA-1 (40 hex chars)
		{name: "SHA-1 lowercase", input: sha1Lower, want: sha1Lower},
		{name: "SHA-1 uppercase", input: strings.ToUpper(sha1Lower), want: sha1Lower},
		{name: "SHA-1 with prefix", input: "sha1:" + sha1Lower, want: sha1Lower},
		{name: "SHA-1 with uppercase prefix", input: "SHA-1:" + sha1Lower, want: sha1Lower},

		// Valid cases - SHA-256 (64 hex chars)
		{name: "SHA-256 lowercase", input: sha256Lower, want: sha256Lower},
		{name: "SHA-256 uppercase", input: strings.ToUpper(sha256Lower), want: sha256Lower},
		{name: "SHA-256 with prefix", input: "sha256:" + sha256Lower, want: sha256Lower},
		{name: "SHA-256 prefixed, formatted with whitespace and filename", input: "  SHA256:" + strings.ToUpper(sha256Lower) + "  ubuntu.iso \n", want: sha256Lower},

		// Valid cases - SHA-512 (128 hex chars)
		{name: "SHA-512 lowercase", input: sha512Lower, want: sha512Lower},
		{name: "SHA-512 uppercase", input: strings.ToUpper(sha512Lower), want: sha512Lower},
		{name: "SHA-512 with prefix", input: "sha512:" + sha512Lower, want: sha512Lower},

		// Valid cases - formatting variations
		{name: "Leading colon", input: ":" + sha256Lower, want: sha256Lower},
		{name: "Surrounding whitespace", input: "   \t " + sha256Lower + "   \n", want: sha256Lower},

		// Invalid cases - Empty/Whitespace
		{name: "Empty string", input: "", wantErr: true},
		{name: "Whitespace only", input: "   \t\n  ", wantErr: true},

		// Invalid cases - Invalid hex characters
		{name: "Non-hex characters in hash", input: "d41d8cd98f00b204e9800998ecf8427g", wantErr: true},
		{name: "Not a hash string", input: "not-a-hash", wantErr: true},
		{name: "Hex with invalid symbol", input: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85!", wantErr: true},

		// Invalid cases - Unsupported lengths
		{name: "Length 31 (short MD5)", input: md5Lower[:31], wantErr: true},
		{name: "Length 33 (long MD5)", input: md5Lower + "0", wantErr: true},
		{name: "Length 39 (short SHA-1)", input: sha1Lower[:39], wantErr: true},
		{name: "Length 41 (long SHA-1)", input: sha1Lower + "0", wantErr: true},
		{name: "Length 63 (short SHA-256)", input: sha256Lower[:63], wantErr: true},
		{name: "Length 65 (long SHA-256)", input: sha256Lower + "0", wantErr: true},
		{name: "Length 127 (short SHA-512)", input: sha512Lower[:127], wantErr: true},
		{name: "Length 129 (long SHA-512)", input: sha512Lower + "0", wantErr: true},

		// Invalid cases - Bad prefix / colon usage
		{name: "Prefix with no hash", input: "sha256:", wantErr: true},
		{name: "Hash followed by colon", input: sha256Lower + ":", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseChecksum(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseChecksum(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("ParseChecksum(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestChecksumError(t *testing.T) {
	err := &ChecksumError{Algo: "SHA-256", Want: "abc", Got: "def"}
	expected := "SHA-256 checksum doesn't match: the file may be damaged or not the one expected"
	if got := err.Error(); got != expected {
		t.Errorf("ChecksumError.Error() = %q, want %q", got, expected)
	}
}

func TestVerifyChecksum(t *testing.T) {
	tmp, err := os.CreateTemp("", "checksum-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	content := []byte("hello world")
	if _, err := tmp.Write(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	// SHA-256 of "hello world"
	sha256Hash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	if err := verifyChecksum(tmp.Name(), sha256Hash); err != nil {
		t.Errorf("verifyChecksum valid lowercase failed: %v", err)
	}

	if err := verifyChecksum(tmp.Name(), strings.ToUpper(sha256Hash)); err != nil {
		t.Errorf("verifyChecksum valid uppercase failed: %v", err)
	}

	badHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := verifyChecksum(tmp.Name(), badHash); err == nil {
		t.Errorf("verifyChecksum should fail for invalid hash")
	}

	if err := verifyChecksum(tmp.Name(), "invalid-algo"); err == nil {
		t.Errorf("verifyChecksum should fail for unknown checksum length")
	}
}

func BenchmarkVerifyChecksum(b *testing.B) {
	tmp, err := os.CreateTemp("", "checksum-bench-*.bin")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	data := make([]byte, 10<<20) // 10 MB
	for i := range data {
		data[i] = byte(i)
	}
	if _, err := tmp.Write(data); err != nil {
		b.Fatal(err)
	}
	tmp.Close()

	h := sha256.New()
	h.Write(data)
	sha256Hash := hex.EncodeToString(h.Sum(nil))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := verifyChecksum(tmp.Name(), sha256Hash); err != nil {
			b.Fatal(err)
		}
	}
}
