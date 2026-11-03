//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// launch starts FluxDM in its own session. On macOS the .app bundle is opened
// with `open` so LaunchServices handles it properly.
func launch(exe string, background bool) error {
	if runtime.GOOS == "darwin" {
		if i := strings.Index(exe, ".app/"); i > 0 {
			bundle := exe[:i+4]
			args := []string{"-g", bundle}
			if background {
				args = append(args, "--args", "--background")
			}
			return exec.Command("open", args...).Start()
		}
	}
	args := []string{}
	if background {
		args = append(args, "--background")
	}
	cmd := exec.Command(filepath.Clean(exe), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
