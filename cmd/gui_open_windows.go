//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

var guiShellDLL = syscall.NewLazyDLL("shell32.dll")
var guiShellProc = guiShellDLL.NewProc("ShellExecuteW")
var guiUser32 = syscall.NewLazyDLL("user32.dll")
var guiMessageBox = guiUser32.NewProc("MessageBoxW")

func guiShellOpen(target string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	item, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ret, _, callErr := guiShellProc.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(item)), 0, 0, 1)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW error %d: %v", ret, callErr)
	}
	return nil
}
func reportGUIError(err error) {
	if err == nil {
		return
	}
	title, _ := syscall.UTF16PtrFromString("SyndiRelay")
	msg, _ := syscall.UTF16PtrFromString(err.Error())
	guiMessageBox.Call(0, uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(title)), 0x10)
}
func openEdgeExtensions() error {
	candidates := []string{}
	if exe, err := exec.LookPath("msedge.exe"); err == nil {
		candidates = append(candidates, exe)
	}
	for _, folder := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if folder != "" {
			candidates = append(candidates, filepath.Join(folder, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		cmd := exec.Command(path, "edge://extensions/")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
		if err := cmd.Start(); err != nil {
			continue
		}
		return cmd.Process.Release()
	}
	return errors.New("Microsoft Edge is not installed; open your browser extension settings manually")
}

func openNativeGUI(mode string) error {
	me, err := os.Executable()
	if err != nil {
		return err
	}
	bin := filepath.Dir(me)
	root := filepath.Dir(bin)
	cli := filepath.Join(bin, "syndirelay.exe")
	if _, err := os.Stat(cli); err != nil {
		return err
	}
	// Register the real CLI/Native Host, never the GUI launcher.
	if _, err := registerNativeHost(cli); err != nil {
		return err
	}
	folder := filepath.Join(root, "extension")
	if _, err := os.Stat(folder); err != nil {
		return err
	}
	if err := guiShellOpen(folder); err != nil {
		return err
	}
	if mode == "--open-ui" {
		// Use shell association instead of spawning PowerShell or cmd.exe.
		return openEdgeExtensions()
	}
	if mode == "--guide-extension" {
		guide := filepath.Join(root, "README_ZH.md")
		if _, err := os.Stat(guide); err == nil {
			return guiShellOpen(guide)
		}
		return nil
	}
	return errors.New("unsupported GUI action")
}
