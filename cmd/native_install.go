package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const blogctlExtensionID = "kbenbblolndleojbmcjcfmkkgfhljmbe"

type nativeHostManifest struct {
	Name           string
	Description    string
	Path           string
	Type           string
	AllowedOrigins []string
}

func nativeHostManifestBytes(executable string) ([]byte, error) {
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("Native Messaging executable must be absolute: %q", executable)
	}
	manifest := map[string]any{
		"name":            nativeHostName,
		"description":     "BlogCTL local bridge launcher",
		"path":            filepath.Clean(executable),
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + blogctlExtensionID + "/"},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func ownedNativeHostManifest(data []byte) bool {
	var manifest map[string]any
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}
	if manifest["name"] != nativeHostName || manifest["type"] != "stdio" {
		return false
	}
	origins, ok := manifest["allowed_origins"].([]any)
	return ok && len(origins) == 1 && origins[0] == "chrome-extension://"+blogctlExtensionID+"/"
}

func writeNativeHostManifest(path, executable string) error {
	data, err := nativeHostManifestBytes(executable)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func runNativeHostSetup(command string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: blogctl %s", command)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	switch command {
	case "install":
		count, err := registerNativeHost(executable)
		if err != nil {
			return err
		}
		fmt.Printf("BlogCTL Native Messaging Host registered for %d browser(s).\nExecutable: %s\nReload the browser extension to reconnect.\n", count, executable)
	case "uninstall":
		count, err := unregisterNativeHost(executable)
		if err != nil {
			return err
		}
		fmt.Printf("BlogCTL Native Messaging Host unregistered from %d browser(s).\nUser Data and the browser extension were preserved.\n", count)
	default:
		return fmt.Errorf("unknown Native Messaging setup command %q", command)
	}
	return nil
}
