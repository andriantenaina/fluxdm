package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"fluxdm/internal/bridge"
	"fluxdm/internal/core"
	"fluxdm/internal/media"
	"fluxdm/internal/store"
)

// ytdlpHostsFn returns the hosts handled by yt-dlp at page level (set by media wiring).
var ytdlpHostsFn = func() []string { return nil }

type bridgeHandler struct{ ui *UI }

func (u *UI) startBridge(dataDir string) {
	srv, err := bridge.Start(dataDir, u.version, &bridgeHandler{ui: u})
	if err != nil {
		log.Printf("bridge: %v", err)
		return
	}
	u.AddBackground(func(stop <-chan struct{}) {
		<-stop
		srv.Close()
	})
}

func headersFrom(referer, origin, cookies, ua string, extra map[string]string) map[string]string {
	h := map[string]string{}
	for k, v := range extra {
		h[k] = v
	}
	if referer != "" {
		h["Referer"] = referer
	}
	if origin != "" {
		h["Origin"] = origin
	}
	if cookies != "" {
		h["Cookie"] = cookies
	}
	if ua != "" {
		h["User-Agent"] = ua
	}
	return h
}

func (b *bridgeHandler) Download(r bridge.DownloadReq) (string, error) {
	req := core.AddRequest{
		URL:      r.URL,
		FileName: r.FileName,
		PageURL:  r.PageURL,
		Size:     r.Size,
		Headers:  headersFrom(r.Referer, "", r.Cookies, r.UserAgent, r.Headers),
		Start:    true,
	}
	s := b.ui.mgr.Settings()
	if r.Silent || !s.ShowAddDialog {
		return b.add(req)
	}
	b.ui.OpenAdd(req, nil, false)
	return "", nil
}

func (b *bridgeHandler) add(req core.AddRequest) (string, error) {
	it, err := b.ui.api.Add(req)
	if err != nil {
		return "", err
	}
	return it.ID, nil
}

func (b *bridgeHandler) Batch(r bridge.BatchReq) ([]string, error) {
	h := headersFrom(r.PageURL, "", r.Cookies, r.UserAgent, nil)
	var reqs []core.AddRequest
	seen := map[string]bool{}
	for _, l := range r.Links {
		if l.URL == "" || seen[l.URL] {
			continue
		}
		seen[l.URL] = true
		reqs = append(reqs, core.AddRequest{URL: l.URL, Title: strings.TrimSpace(l.Text), PageURL: r.PageURL, Headers: h, Size: l.Size})
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	b.ui.OpenAdd(core.AddRequest{PageURL: r.PageURL}, reqs, false)
	return nil, nil // the batch dialog creates the items
}

func (b *bridgeHandler) Video(r bridge.VideoReq) (string, error) {
	h := headersFrom(r.Referer, r.Origin, r.Cookies, r.UserAgent, r.Headers)
	req := core.AddRequest{
		URL:     r.URL,
		PageURL: r.PageURL,
		Title:   r.Title,
		Headers: h,
		Start:   true,
		Options: map[string]any{},
	}
	switch r.Kind {
	case "hls":
		req.Kind = store.KindHLS
	case "dash":
		req.Kind = store.KindDASH
	case "page":
		req.Kind = store.KindYtdlp
		if req.URL == "" {
			req.URL = r.PageURL
		}
	default:
		req.Kind = store.KindHTTP
		req.Category = b.ui.mgr.Settings().CategoryName("video")
	}
	if r.FileName != "" {
		req.FileName = r.FileName
	}
	switch {
	case r.AudioOnly || r.Quality == "audio":
		req.Options["audioOnly"] = true
		req.Options["output"] = "mp3"
		req.Category = b.ui.mgr.Settings().CategoryName("audio")
	case strings.HasPrefix(r.Quality, "rep:"):
		req.Options["videoRep"] = strings.TrimPrefix(r.Quality, "rep:")
	case strings.HasPrefix(r.Quality, "http") && req.Kind == store.KindHLS:
		req.Options["variantUrl"] = r.Quality
	}
	if r.Height > 0 {
		req.Options["maxHeight"] = r.Height
	}
	if len(r.Variants) > 0 {
		vs := make([]any, 0, len(r.Variants))
		for _, v := range r.Variants {
			vs = append(vs, map[string]any{"url": v.URL, "width": v.Width, "height": v.Height, "bandwidth": v.Bandwidth, "label": v.Label, "mime": v.Mime, "size": v.Size, "kind": v.StreamKind})
		}
		req.Options["variants"] = vs
	}
	if r.Quick {
		return b.add(req)
	}
	b.ui.OpenAdd(req, nil, true)
	return "", nil
}

// Inspect tells the extension the real title, qualities, sizes and output format.
func (b *bridgeHandler) Inspect(r bridge.InspectReq) bridge.InspectResp {
	if b.ui.svc == nil || b.ui.svc.media == nil {
		return bridge.InspectResp{Error: "non prêt"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	h := headersFrom(r.Referer, r.Origin, r.Cookies, r.UserAgent, r.Headers)
	target := r.URL
	if r.Kind == "page" && target == "" {
		target = r.PageURL
	}
	in, err := b.ui.svc.media.Inspect(ctx, media.InspectInput{Kind: r.Kind, URL: target, Title: r.Title, FileName: r.FileName, Mime: r.Mime, Size: r.Size, Headers: h})
	if err != nil {
		// still give a clean name so the UI never shows "master.m3u8"
		ext := media.ExtForMime(r.Mime)
		if ext == "" {
			ext = b.ui.mgr.Settings().HLSOutput
		}
		return bridge.InspectResp{Title: media.CleanTitle(r.Title), OutputExt: ext, FileName: media.FinalName(r.Title, "", ext), Error: err.Error()}
	}
	resp := bridge.InspectResp{Title: in.Title, Duration: in.Duration, DefaultQuality: in.DefaultQuality,
		OutputExt: in.OutputExt, FileName: in.FileName, Live: in.Live, Thumbnail: in.Thumbnail}
	for _, q := range in.Qualities {
		resp.Qualities = append(resp.Qualities, bridge.Quality{ID: q.ID, Label: q.Label, Height: q.Height, Bandwidth: q.Bandwidth, SizeEstimate: q.SizeEstimate, Ext: q.Ext})
	}
	return resp
}

func (b *bridgeHandler) ExtSettings() bridge.ExtSettings {
	s := b.ui.mgr.Settings()
	return bridge.ExtSettings{
		Version:        b.ui.version,
		CaptureExts:    nonNil(s.CaptureExts),
		CaptureMinSize: s.CaptureMinSize,
		SkipSites:      nonNil(s.SkipSites),
		VideoButton:    s.VideoButton,
		VideoSkipSites: nonNil(s.ExtVideoSkipSites),
		CaptureEnabled: !s.CaptureDisabled,
		Enabled:        true,
		YtdlpSites:     nonNil(ytdlpHostsFn()),
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// cleanList trims, lowercases and de-duplicates extensions / host names.
func cleanList(in []string, trimDot bool) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if trimDot {
			v = strings.TrimPrefix(v, ".")
		}
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// SaveExtSettings applies the extension's options page to the app settings.
func (b *bridgeHandler) SaveExtSettings(p bridge.ExtSettingsPatch) (bridge.ExtSettings, error) {
	s := b.ui.mgr.Settings()
	if p.CaptureExts != nil {
		s.CaptureExts = cleanList(*p.CaptureExts, true)
	}
	if p.CaptureMinSize != nil {
		s.CaptureMinSize = max(*p.CaptureMinSize, 0) // bytes
	}
	if p.SkipSites != nil {
		s.SkipSites = cleanList(*p.SkipSites, false)
	}
	if p.VideoButton != nil {
		s.VideoButton = *p.VideoButton
	}
	if p.VideoSkipSites != nil {
		s.ExtVideoSkipSites = cleanList(*p.VideoSkipSites, false)
	}
	if p.CaptureEnabled != nil {
		s.CaptureDisabled = !*p.CaptureEnabled
	}
	if err := b.ui.mgr.SetSettings(s); err != nil {
		return bridge.ExtSettings{}, err
	}
	return b.ExtSettings(), nil
}

// Active lists running downloads first, then up to 5 queued and the 5 most
// recently completed (at most 15).
func (b *bridgeHandler) Active() bridge.ActiveResp {
	items := b.ui.mgr.List()
	var running, queued, done []*store.Item
	for _, it := range items {
		switch it.Status {
		case store.StatusDownloading, store.StatusProcessing:
			running = append(running, it)
		case store.StatusQueued:
			queued = append(queued, it)
		case store.StatusCompleted:
			done = append(done, it)
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
		if queued[i].Priority != queued[j].Priority {
			return queued[i].Priority > queued[j].Priority
		}
		return queued[i].SortKey() < queued[j].SortKey()
	})
	sort.SliceStable(done, func(i, j int) bool {
		return done[i].CompletedAt != nil && (done[j].CompletedAt == nil || done[i].CompletedAt.After(*done[j].CompletedAt))
	})
	tot := b.ui.mgr.Totals()
	resp := bridge.ActiveResp{Items: []bridge.ActiveItem{}, Speed: tot.Speed, Active: tot.Active}
	add := func(it *store.Item) {
		ai := bridge.ActiveItem{ID: it.ID, FileName: it.FileName, Title: it.Title, Status: it.Status,
			Size: it.Size, Downloaded: it.Downloaded, Kind: it.Kind, ETA: -1}
		if p := b.ui.mgr.LatestProgress(it.ID); p != nil {
			ai.Downloaded, ai.Speed, ai.ETA, ai.ETAConfidence = p.Downloaded, p.Speed, p.ETA, p.ETAConfidence
			if p.Size > 0 {
				ai.Size = p.Size
			}
		}
		if it.Status == store.StatusCompleted {
			ai.ETA, ai.ETAConfidence = 0, 1
			if it.CompletedAt != nil {
				ai.CompletedAt = it.CompletedAt.Format(time.RFC3339)
			}
		}
		resp.Items = append(resp.Items, ai)
	}
	for _, it := range running {
		if len(resp.Items) < 15 {
			add(it)
		}
	}
	for i, it := range queued {
		if i < 5 && len(resp.Items) < 15 {
			add(it)
		}
	}
	for i, it := range done {
		if i < 5 && len(resp.Items) < 15 {
			add(it)
		}
	}
	return resp
}

// Show brings FluxDM to the front, on an item's progress window when id is set.
func (b *bridgeHandler) Show(id string) error {
	if id != "" {
		if _, err := b.ui.mgr.Get(id); err != nil {
			return err
		}
	}
	b.ui.ShowMain()
	if id != "" {
		b.ui.OpenProgress(id)
	}
	return nil
}

// Control pauses, resumes or cancels an item.
func (b *bridgeHandler) Control(id, action string) error {
	if _, err := b.ui.mgr.Get(id); err != nil {
		return err
	}
	switch action {
	case "pause":
		go b.ui.mgr.Pause(id)
		return nil
	case "resume":
		return b.ui.mgr.Start(id)
	case "cancel":
		go b.ui.mgr.Cancel([]string{id})
		return nil
	}
	return fmt.Errorf("action inconnue : %s", action)
}
