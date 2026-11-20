//go:build !windows

package main

import "runtime"

// glassSupported: macOS translucency works on every supported version (12+).
func glassSupported() bool { return runtime.GOOS == "darwin" }
