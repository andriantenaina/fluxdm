//go:build !windows

package main

// systemDark is not detected outside Windows: the webview applies the theme.
func systemDark() bool { return true }

func framelessSupported() bool { return false }
