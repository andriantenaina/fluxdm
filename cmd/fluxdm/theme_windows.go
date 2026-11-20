//go:build windows

package main

import "golang.org/x/sys/windows/registry"

// systemDark reports whether Windows apps use the dark theme.
func systemDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}

// framelessSupported: the custom title bar is implemented for Windows only
// (macOS keeps its native inset title bar).
func framelessSupported() bool { return true }
