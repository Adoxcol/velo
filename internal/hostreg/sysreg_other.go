//go:build !windows

package hostreg

import "errors"

func registerKey(keyPath, val string) error  { return nil }
func readKey(keyPath string) (string, error) { return "", errors.New("not supported") }
func deleteKey(keyPath string)               {}
