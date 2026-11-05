//go:build !windows

package main

import "errors"

func openNativeGUI(mode string) error { return errors.New("native GUI entry is Windows-only") }
func reportGUIError(err error)        {}
