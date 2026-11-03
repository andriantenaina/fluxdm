//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"fluxdm/internal/bridge"
)

// registerIntegration writes the native messaging host manifest into each
// Chromium browser's NativeMessagingHosts folder (macOS / Linux). The
// fluxdm:// protocol is declared in Info.plist on macOS.
func registerIntegration(dataDir string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	host := filepath.Join(filepath.Dir(exe), "fluxdm-host")
	if _, err := os.Stat(host); err != nil {
		return
	}
	home, _ := os.UserHomeDir()
	var dirs []string
	if runtime.GOOS == "darwin" {
		as := filepath.Join(home, "Library", "Application Support")
		dirs = []string{
			filepath.Join(as, "Google", "Chrome"),
			filepath.Join(as, "Google", "Chrome Beta"),
			filepath.Join(as, "Chromium"),
			filepath.Join(as, "Microsoft Edge"),
			filepath.Join(as, "BraveSoftware", "Brave-Browser"),
			filepath.Join(as, "Vivaldi"),
			filepath.Join(as, "com.operasoftware.Opera"),
			filepath.Join(as, "com.operasoftware.OperaGX"),
			filepath.Join(as, "Opera"),
		}
	} else {
		cfg := filepath.Join(home, ".config")
		dirs = []string{
			filepath.Join(cfg, "google-chrome"),
			filepath.Join(cfg, "chromium"),
			filepath.Join(cfg, "microsoft-edge"),
			filepath.Join(cfg, "BraveSoftware", "Brave-Browser"),
			filepath.Join(cfg, "vivaldi"),
			filepath.Join(cfg, "opera"),
		}
	}
	manifest := bridge.NativeManifest(host)
	for i, d := range dirs {
		// Always write for Chrome (Opera falls back to it); others only if installed.
		if _, err := os.Stat(d); err != nil && i != 0 {
			continue
		}
		nd := filepath.Join(d, "NativeMessagingHosts")
		if os.MkdirAll(nd, 0o755) == nil {
			_ = os.WriteFile(filepath.Join(nd, bridge.HostName+".json"), manifest, 0o644)
		}
	}
	_ = dataDir
}
