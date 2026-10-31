package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

var browserRegistryKeys = []string{
	"Software\\Google\\Chrome\\NativeMessagingHosts\\" + nativeHostName,
	"Software\\Microsoft\\Edge\\NativeMessagingHosts\\" + nativeHostName,
}

func windowsNativeHostManifestPath(executable string) string {
	return filepath.Join(filepath.Dir(executable), "Data", nativeHostName+".json")
}

func registerNativeHost(executable string) (int, error) {
	path := windowsNativeHostManifestPath(executable)
	if err := writeNativeHostManifest(path, executable); err != nil {
		return 0, fmt.Errorf("write manifest: %w", err)
	}
	count := 0
	for _, name := range browserRegistryKeys {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, name, registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			return count, fmt.Errorf("create Native Messaging registry key %s: %w", name, err)
		}
		if err := key.SetStringValue("", path); err != nil {
			key.Close()
			return count, fmt.Errorf("register %s: %w", name, err)
		}
		value, _, err := key.GetStringValue("")
		closeErr := key.Close()
		if err != nil {
			return count, err
		}
		if closeErr != nil {
			return count, closeErr
		}
		if !strings.EqualFold(filepath.Clean(value), filepath.Clean(path)) {
			return count, fmt.Errorf("Native Messaging registry mismatch: %s", name)
		}
		count++
	}
	return count, nil
}

func unregisterNativeHost(executable string) (int, error) {
	count := 0
	manifests := map[string]struct{}{}
	for _, name := range browserRegistryKeys {
		key, err := registry.OpenKey(registry.CURRENT_USER, name, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, err
		}
		path, _, readErr := key.GetStringValue("")
		closeErr := key.Close()
		if readErr != nil {
			return count, readErr
		}
		if closeErr != nil {
			return count, closeErr
		}
		if !strings.EqualFold(filepath.Base(path), nativeHostName+".json") {
			return count, fmt.Errorf("refusing to unregister unknown manifest: %s", path)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			if !ownedNativeHostManifest(data) {
				return count, fmt.Errorf("refusing to unregister unknown Native Messaging manifest: %s", path)
			}
			manifests[path] = struct{}{}
		} else if !errors.Is(err, os.ErrNotExist) {
			return count, err
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, name); err != nil {
			return count, fmt.Errorf("unregister %s: %w", name, err)
		}
		count++
	}
	for path := range manifests {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return count, fmt.Errorf("remove manifest %s: %w", path, err)
		}
	}
	return count, nil
}
