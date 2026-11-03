// Command fluxdm is the FluxDM desktop application (Wails v3).
//
// Usage:
//
//	fluxdm                     start the app (or focus the running one)
//	fluxdm add <url> [...]     send URLs to the running app
//	fluxdm fluxdm://add?url=…  protocol handler
//	fluxdm file.torrent        open a torrent file
package main

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"fluxdm/frontend"
	"fluxdm/internal/core"
)

// Version is set at build time with -ldflags "-X main.Version=x.y.z".
var Version = "0.1.0"

const appID = "com.fluxdm.app"

func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, "FluxDM")
}

func main() {
	dir := dataDir()
	_ = os.MkdirAll(dir, 0o755)
	if f, err := os.OpenFile(filepath.Join(dir, "fluxdm.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}

	mgr, err := core.NewManager(dir)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	ui := &UI{mgr: mgr, version: Version}
	api := &API{mgr: mgr, ui: ui}
	ui.api = api
	registerRunners(mgr, ui)

	app := application.New(application.Options{
		Name:        "FluxDM",
		Description: "Gestionnaire de téléchargements",
		Icon:        appIcon,
		LogLevel:    slog.LevelWarn,
		Services:    []application.Service{application.NewService(api)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Dist())},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: appID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				ui.handleArgs(data.Args[min(1, len(data.Args)):], true)
			},
		},
		FileAssociations: []string{".torrent"},
		OnShutdown: func() {
			ui.shutdown()
			mgr.Close()
		},
	})
	ui.app = app

	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		ui.handleArgs([]string{e.Context().URL()}, true)
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		ui.handleArgs([]string{e.Context().Filename()}, true)
	})

	ui.createMainWindow()
	ui.createTray()
	ui.startEventForwarding()
	ui.startClipboardMonitor()
	ui.startBridge(dir)
	ui.startBackground()

	go registerIntegration(dir)
	go func() {
		mgr.Resume()
		ui.handleArgs(os.Args[1:], false)
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
