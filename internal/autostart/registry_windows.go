//go:build windows

package autostart

import (
	"errors"
	"io/fs"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func defaultRegistry() Registry { return runKey{path: runKeyPath} }

// runKey is a string value store under HKEY_CURRENT_USER\path.
type runKey struct{ path string }

func (k runKey) Get(name string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, k.path, registry.QUERY_VALUE)
	if err != nil {
		return "", notExist(err)
	}
	defer key.Close()
	v, _, err := key.GetStringValue(name)
	return v, notExist(err)
}

func (k runKey) Set(name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, k.path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(name, value)
}

func (k runKey) Delete(name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, k.path, registry.SET_VALUE)
	if err == nil {
		defer key.Close()
		err = key.DeleteValue(name)
	}
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func notExist(err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return fs.ErrNotExist
	}
	return err
}
