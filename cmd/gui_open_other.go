//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open the native extension folder without invoking PowerShell or Bash.
func openNativeGUI(mode string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	directory := filepath.Dir(exe)
	root := filepath.Dir(directory)
	if filepath.Base(directory) == "MacOS" &&
		filepath.Base(filepath.Dir(directory)) == "Contents" {
		root = filepath.Clean(filepath.Join(directory, "../../.."))
	}
	cli := filepath.Join(root, "bin", "syndirelay")
	if _, err := os.Stat(cli); err != nil {
		return err
	}
	if _, err := registerNativeHost(cli); err != nil {
		return err
	}
	ext := filepath.Join(root, "extension")
	if _, err := os.Stat(ext); err != nil {
		return err
	}
	target := ext
	if mode == "--guide-extension" {
		target = filepath.Join(root, "README_ZH.md")
	}
	if mode != "--open-ui" && mode != "--guide-extension" {
		return errors.New("unsupported native entry")
	}
	command := "gio"
	args := []string{"open", target}
	if runtime.GOOS == "darwin" {
		command = "open"
		args = []string{target}
	}
	cmd := exec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stdout, "Native host registered. Load unpacked extension:", ext)
		return nil
	}
	return cmd.Process.Release()
}
func reportGUIError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
