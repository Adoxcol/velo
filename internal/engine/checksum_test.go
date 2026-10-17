package engine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksumError(t *testing.T) {
	err := &ChecksumError{Algo: "SHA-256", Want: "abc", Got: "def"}
	wantMsg := "SHA-256 checksum doesn't match: the file may be damaged or not the one expected"
	if err.Error() != wantMsg {
		t.Errorf("got %q, want %q", err.Error(), wantMsg)
	}
}

func TestChecksumAlgo(t *testing.T) {
	tests := []struct {
		sum  string
		want string
	}{
		{strings.Repeat("a", 32), "MD5"},
		{strings.Repeat("a", 40), "SHA-1"},
		{strings.Repeat("a", 64), "SHA-256"},
		{strings.Repeat("a", 128), "SHA-512"},
		{"", ""},
		{strings.Repeat("a", 10), ""},
		{strings.Repeat("a", 33), ""},
		{strings.Repeat("a", 256), ""},
	}
	for _, tt := range tests {
		if got := checksumAlgo(tt.sum); got != tt.want {
			t.Errorf("checksumAlgo(%d chars): got %q, want %q", len(tt.sum), got, tt.want)
		}
	}
}

func TestParseChecksum(t *testing.T) {
	md5Str := strings.Repeat("a", 32)
	sha1Str := strings.Repeat("b", 40)
	sha256Str := strings.Repeat("c", 64)
	sha512Str := strings.Repeat("d", 128)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"MD5 raw", md5Str, md5Str, false},
		{"MD5 uppercase with prefix", "MD5:" + strings.ToUpper(md5Str), md5Str, false},
		{"SHA-1 raw with whitespace", "  " + sha1Str + "  ", sha1Str, false},
		{"SHA-256 formatted with filename", sha256Str + "  some_file.iso", sha256Str, false},
		{"SHA-512 with colon prefix and filename", "SHA512:" + sha512Str + "  data.bin", sha512Str, false},
		{"Invalid hex char", strings.Repeat("z", 32), "", true},
		{"Invalid length", strings.Repeat("a", 30), "", true},
		{"Empty string", "", "", true},
		{"Colon only", ":", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChecksum(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseChecksum(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseChecksum(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("testing checksum verification content")
	dir := t.TempDir()
	filePath := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	md5Hash := md5.Sum(data)
	md5Sum := hex.EncodeToString(md5Hash[:])

	sha1Hash := sha1.Sum(data)
	sha1Sum := hex.EncodeToString(sha1Hash[:])

	sha256Hash := sha256.Sum256(data)
	sha256Sum := hex.EncodeToString(sha256Hash[:])

	sha512Hash := sha512.Sum512(data)
	sha512Sum := hex.EncodeToString(sha512Hash[:])

	t.Run("Valid MD5", func(t *testing.T) {
		if err := verifyChecksum(filePath, md5Sum); err != nil {
			t.Errorf("verifyChecksum failed for MD5: %v", err)
		}
	})

	t.Run("Valid SHA-1", func(t *testing.T) {
		if err := verifyChecksum(filePath, sha1Sum); err != nil {
			t.Errorf("verifyChecksum failed for SHA-1: %v", err)
		}
	})

	t.Run("Valid SHA-256", func(t *testing.T) {
		if err := verifyChecksum(filePath, sha256Sum); err != nil {
			t.Errorf("verifyChecksum failed for SHA-256: %v", err)
		}
	})

	t.Run("Valid SHA-512", func(t *testing.T) {
		if err := verifyChecksum(filePath, sha512Sum); err != nil {
			t.Errorf("verifyChecksum failed for SHA-512: %v", err)
		}
	})

	t.Run("Checksum mismatch", func(t *testing.T) {
		wrongSum := strings.Repeat("0", 64)
		err := verifyChecksum(filePath, wrongSum)
		if err == nil {
			t.Fatal("expected error for checksum mismatch, got nil")
		}
		var ce *ChecksumError
		if !errors.As(err, &ce) {
			t.Fatalf("expected ChecksumError, got %T: %v", err, err)
		}
		if ce.Algo != "SHA-256" || ce.Want != wrongSum || ce.Got != sha256Sum {
			t.Errorf("unexpected ChecksumError fields: %+v", ce)
		}
	})

	t.Run("Non-existent file", func(t *testing.T) {
		nonExistent := filepath.Join(dir, "does_not_exist.txt")
		err := verifyChecksum(nonExistent, sha256Sum)
		if err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected os.ErrNotExist, got %v", err)
		}
	})

	t.Run("Unknown algorithm", func(t *testing.T) {
		invalidLengthSum := strings.Repeat("a", 50)
		err := verifyChecksum(filePath, invalidLengthSum)
		if err == nil {
			t.Fatal("expected error for unknown checksum algorithm, got nil")
		}
		if !strings.Contains(err.Error(), "unknown checksum") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
