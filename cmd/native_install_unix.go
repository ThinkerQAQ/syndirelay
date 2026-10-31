//go:build linux || darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func nativeHostManifestPaths(home string) []string {
	var dirs []string
	switch runtime.GOOS {
	case "linux":
		dirs = []string{".config/google-chrome/NativeMessagingHosts", ".config/chromium/NativeMessagingHosts", ".config/microsoft-edge/NativeMessagingHosts"}
	case "darwin":
		dirs = []string{"Library/Application Support/Google/Chrome/NativeMessagingHosts", "Library/Application Support/Chromium/NativeMessagingHosts", "Library/Application Support/Microsoft Edge/NativeMessagingHosts"}
	}
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(home, dir, nativeHostName+".json"))
	}
	return paths
}

func registerNativeHost(executable string) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, path := range nativeHostManifestPaths(home) {
		if err := writeNativeHostManifest(path, executable); err != nil {
			return count, fmt.Errorf("register %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

func unregisterNativeHost(executable string) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, path := range nativeHostManifestPaths(home) {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", path, err)
		}
		if !ownedNativeHostManifest(data) {
			return count, fmt.Errorf("refusing to remove unknown Native Messaging manifest %s", path)
		}
		if err := os.Remove(path); err != nil {
			return count, fmt.Errorf("unregister %s: %w", path, err)
		}
		count++
	}
	return count, nil
}
