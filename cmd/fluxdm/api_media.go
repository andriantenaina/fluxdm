package main

import (
	"context"
	"errors"
	"time"

	"fluxdm/internal/grabber"
	"fluxdm/internal/media"
	"fluxdm/internal/scheduler"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/torrent"
)

// AnalyzeVideo lists the qualities of a stream / page for the video dialog.
// kind: hls | dash | ytdlp | hoster | http | "" (auto).
func (a *API) AnalyzeVideo(kind, rawURL string, headers map[string]string, playlist bool) (*media.VideoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return a.ui.svc.media.Analyze(ctx, kind, rawURL, headers, playlist)
}

// DetectKind returns how a URL would be downloaded.
func (a *API) DetectKind(rawURL string) string { return a.mgr.DetectKind(rawURL) }

// Tools returns yt-dlp / ffmpeg / deno status.
func (a *API) Tools() []sidecars.Status { return a.ui.svc.tools.Statuses() }

// InstallTool downloads or updates a tool in the background (progress: "fdm:tool").
func (a *API) InstallTool(name string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = a.ui.svc.tools.Update(ctx, name)
	}()
}

// Hosters lists supported video hosts and the rules version.
func (a *API) Hosters() map[string]any {
	return map[string]any{"names": a.ui.svc.hosters.Names(), "version": a.ui.svc.hosters.Version(), "ytdlp": media.YtdlpHosts}
}

// UpdateRules downloads newer hoster rules now.
func (a *API) UpdateRules() (bool, error) {
	s := a.mgr.Settings()
	if s.RulesURL == "" {
		return false, errors.New("aucune adresse de mise à jour des règles n'est configurée")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return a.ui.svc.hosters.Update(ctx, s.RulesURL)
}

// Scheduler returns the scheduler configuration and counters.
func (a *API) Scheduler() scheduler.Status { return a.ui.svc.sched.Status() }

// SaveScheduler stores the scheduler configuration.
func (a *API) SaveScheduler(c scheduler.Config) error { return a.ui.svc.sched.Save(c) }

// GrabberProjects lists site grabber projects.
func (a *API) GrabberProjects() []grabber.Project { return a.ui.svc.grab.Projects() }

// SaveGrabber creates/updates a project.
func (a *API) SaveGrabber(p grabber.Project) (grabber.Project, error) { return a.ui.svc.grab.Save(p) }

// DeleteGrabber removes a project.
func (a *API) DeleteGrabber(id string) error { return a.ui.svc.grab.Delete(id) }

// StartGrabber runs a project (progress: "fdm:grabber").
func (a *API) StartGrabber(id string) error { return a.ui.svc.grab.Start(id) }

// StopGrabber stops a running project.
func (a *API) StopGrabber(id string) { a.ui.svc.grab.Stop(id) }

// InspectTorrent reads a .torrent file / magnet before adding it.
func (a *API) InspectTorrent(src string) (*torrent.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return a.ui.svc.torrent.Inspect(ctx, src, nil)
}

// ChooseTorrent shows a file picker for .torrent files.
func (a *API) ChooseTorrent() (string, error) {
	return a.ChooseFile("Ouvrir un fichier torrent", "Torrent", "*.torrent")
}
