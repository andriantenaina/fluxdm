package main

import (
	"log"
	"strings"

	"fluxdm/internal/bridge"
	"fluxdm/internal/core"
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

func (b *bridgeHandler) Download(r bridge.DownloadReq) error {
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
		_, err := b.ui.api.Add(req)
		return err
	}
	b.ui.OpenAdd(req, nil, false)
	return nil
}

func (b *bridgeHandler) Batch(r bridge.BatchReq) error {
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
		return nil
	}
	b.ui.OpenAdd(core.AddRequest{PageURL: r.PageURL}, reqs, false)
	return nil
}

func (b *bridgeHandler) Video(r bridge.VideoReq) error {
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
		req.Category = "Vidéos"
	}
	if len(r.Variants) > 0 {
		vs := make([]any, 0, len(r.Variants))
		for _, v := range r.Variants {
			vs = append(vs, map[string]any{"url": v.URL, "width": v.Width, "height": v.Height, "bandwidth": v.Bandwidth, "label": v.Label, "mime": v.Mime, "size": v.Size, "kind": v.StreamKind})
		}
		req.Options["variants"] = vs
	}
	if r.Quick {
		_, err := b.ui.api.Add(req)
		return err
	}
	b.ui.OpenAdd(req, nil, true)
	return nil
}

func (b *bridgeHandler) ExtSettings() bridge.ExtSettings {
	s := b.ui.mgr.Settings()
	return bridge.ExtSettings{
		Version:        b.ui.version,
		CaptureExts:    s.CaptureExts,
		CaptureMinSize: s.CaptureMinSize,
		SkipSites:      s.SkipSites,
		VideoButton:    s.VideoButton,
		Enabled:        true,
		YtdlpSites:     ytdlpHostsFn(),
	}
}
