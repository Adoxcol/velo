//go:build windows

package hostreg

import "golang.org/x/sys/windows/registry"

func registerKey(keyPath, val string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("", val)
}

func readKey(keyPath string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return v, err
}

func deleteKey(keyPath string) {
	registry.DeleteKey(registry.CURRENT_USER, keyPath)
}
