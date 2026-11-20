//go:build windows

package main

import "golang.org/x/sys/windows"

// glassSupported reports whether Mica/Acrylic backdrops are available
// (Windows 11 = build 22000+). Windows 10 keeps an opaque window.
func glassSupported() bool {
	return windows.RtlGetVersion().BuildNumber >= 22000
}
