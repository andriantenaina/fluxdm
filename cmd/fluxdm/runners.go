package main

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/extractors"
	"fluxdm/internal/ftpdl"
	"fluxdm/internal/grabber"
	"fluxdm/internal/media"
	"fluxdm/internal/scheduler"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
	"fluxdm/internal/torrent"
)

// services groups the optional modules used by the API.
type services struct {
	tools   *sidecars.Manager
	media   *media.Runtime
	hosters *extractors.Engine
	sched   *scheduler.Scheduler
	grab    *grabber.Grabber
	torrent *torrent.Service
}

// registerRunners wires every download kind and background service.
func registerRunners(m *core.Manager, ui *UI) {
	client := func() *http.Client { return m.HTTPClient() }
	cfg := func() engine.Config { return m.EngineConfig() }

	tools := sidecars.New(filepath.Join(m.DataDir, "tools"), client, cfg)
	tools.HideWindow = core.HideWindow
	tools.OnProgress = func(p sidecars.Progress) {
		if ui.app != nil {
			ui.app.Event.Emit("fdm:tool", p)
		}
	}

	hosters := extractors.New(m.DataDir, client, func() string { return m.EngineConfig().UserAgent })
	media.Hosters = hosters

	rt := media.New(m, tools)
	rt.Register(nil)
	m.Register(store.KindFTP, ftpdl.Runner)

	ts := torrent.New(m, m.DataDir)
	m.Register(store.KindTorrent, ts.Runner)
	m.Subscribe(func(ev core.Event) {
		if ev.Type == "removed" {
			ts.StopSeeding(ev.ID)
		}
	})

	sched := scheduler.New(m)
	sched.Quit = func() {
		if ui.app != nil {
			ui.app.Quit()
		}
	}

	grab := grabber.New(m)
	grab.OnStatus = func(s grabber.RunStatus) {
		if ui.app != nil {
			ui.app.Event.Emit("fdm:grabber", s)
		}
	}

	ytdlpHostsFn = func() []string { return media.YtdlpHosts }

	ui.svc = &services{tools: tools, media: rt, hosters: hosters, sched: sched, grab: grab, torrent: ts}

	ui.AddBackground(sched.Run)
	ui.AddBackground(grab.Periodic)
	ui.AddBackground(func(stop <-chan struct{}) {
		<-stop
		ts.Close()
	})
	// daily maintenance: yt-dlp update + hoster rules update
	ui.AddBackground(func(stop <-chan struct{}) {
		first := time.NewTimer(2 * time.Minute)
		defer first.Stop()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		run := func() {
			s := m.Settings()
			if !s.AutoUpdateTools {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := tools.AutoUpdate(ctx); err != nil {
				log.Printf("yt-dlp update: %v", err)
			}
			if _, err := hosters.Update(ctx, s.RulesURL); err != nil {
				log.Printf("rules update: %v", err)
			}
		}
		for {
			select {
			case <-stop:
				return
			case <-first.C:
				run()
			case <-t.C:
				run()
			}
		}
	})
}
