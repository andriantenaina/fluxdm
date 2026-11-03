//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"fluxdm/internal/bridge"
)

// Browsers whose native messaging registry location we fill. Opera reads the
// Google Chrome key; Edge has its own; Brave/Vivaldi/Chromium use theirs.
var nativeHostKeys = []string{
	`Software\Google\Chrome\NativeMessagingHosts\`,
	`Software\Microsoft\Edge\NativeMessagingHosts\`,
	`Software\Chromium\NativeMessagingHosts\`,
	`Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\`,
	`Software\Vivaldi\NativeMessagingHosts\`,
	`Software\Opera Software\NativeMessagingHosts\`,
}

// registerIntegration registers the native messaging host and the fluxdm://
// protocol for the current user (no admin rights needed). Idempotent.
func registerIntegration(dataDir string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	host := filepath.Join(filepath.Dir(exe), "fluxdm-host.exe")
	if _, err := os.Stat(host); err == nil {
		mdir := filepath.Join(dataDir, "native")
		_ = os.MkdirAll(mdir, 0o755)
		manifest := filepath.Join(mdir, bridge.HostName+".json")
		if err := os.WriteFile(manifest, bridge.NativeManifest(host), 0o644); err != nil {
			log.Printf("native manifest: %v", err)
		} else {
			for _, k := range nativeHostKeys {
				setDefault(k+bridge.HostName, manifest)
			}
		}
	}
	// fluxdm:// protocol
	setDefault(`Software\Classes\fluxdm`, "URL:FluxDM Protocol")
	setValue(`Software\Classes\fluxdm`, "URL Protocol", "")
	setDefault(`Software\Classes\fluxdm\DefaultIcon`, `"`+exe+`",0`)
	setDefault(`Software\Classes\fluxdm\shell\open\command`, `"`+exe+`" "%1"`)
}

func setDefault(path, value string) { setValue(path, "", value) }

func setValue(path, name, value string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		log.Printf("registry %s: %v", path, err)
		return
	}
	defer k.Close()
	_ = k.SetStringValue(name, value)
}
