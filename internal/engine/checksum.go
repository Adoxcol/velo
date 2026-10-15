package engine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// ChecksumError means the finished file doesn't match the expected checksum.
type ChecksumError struct{ Algo, Want, Got string }

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("%s checksum doesn't match: the file may be damaged or not the one expected", e.Algo)
}

// ParseChecksum accepts a hex MD5, SHA-1, SHA-256 or SHA-512, optionally
// prefixed like "sha256:" or followed by a file name (as in .sha256 files),
// and returns it normalised. The algorithm is told by its length.
func ParseChecksum(s string) (string, error) {
	s = strings.TrimSpace(s)
	if f := strings.Fields(s); len(f) > 0 {
		s = f[0]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ToLower(s)
	if _, err := hex.DecodeString(s); err != nil || checksumAlgo(s) == "" {
		return "", errors.New("checksum must be an MD5, SHA-1, SHA-256 or SHA-512 in hex")
	}
	return s, nil
}

func checksumAlgo(sum string) string {
	switch len(sum) {
	case 32:
		return "MD5"
	case 40:
		return "SHA-1"
	case 64:
		return "SHA-256"
	case 128:
		return "SHA-512"
	}
	return ""
}

func verifyChecksum(path, want string) error {
	var h hash.Hash
	algo := checksumAlgo(want)
	switch algo {
	case "MD5":
		h = md5.New()
	case "SHA-1":
		h = sha1.New()
	case "SHA-256":
		h = sha256.New()
	case "SHA-512":
		h = sha512.New()
	default:
		return fmt.Errorf("unknown checksum %q", want)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return &ChecksumError{Algo: algo, Want: want, Got: got}
	}
	return nil
}
