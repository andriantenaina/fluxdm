package media

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
)

// New creates the media runtime.
func New(m *core.Manager, tools *sidecars.Manager) *Runtime {
	return &Runtime{M: m, Tools: tools}
}

// Register installs the HLS, DASH and yt-dlp runners and URL classification.
func (rt *Runtime) Register(extraClassify func(string) string) {
	rt.M.Register(store.KindHLS, rt.RunHLS)
	rt.M.Register(store.KindDASH, rt.RunDASH)
	rt.M.Register(store.KindYtdlp, rt.RunYtdlp)
	rt.M.Register(store.KindHoster, rt.RunHoster)
	rt.M.Classify = func(raw string) string {
		if extraClassify != nil {
			if k := extraClassify(raw); k != "" {
				return k
			}
		}
		if k := core.DetectKind(raw); k != store.KindHTTP {
			return "" // direct .m3u8/.mpd/magnet... keep default detection
		}
		if Hosters != nil && Hosters.Match(raw) != nil && !looksLikeMedia(raw) {
			return store.KindHoster
		}
		if IsYtdlpHost(raw) {
			return store.KindYtdlp
		}
		return ""
	}
}

// Analyze returns the available qualities for the video dialog.
// kind: hls | dash | ytdlp | http (direct file) — empty = auto.
func (rt *Runtime) Analyze(ctx context.Context, kind, rawURL string, headers map[string]string, playlist bool) (*VideoInfo, error) {
	if kind == "" {
		kind = rt.M.DetectKind(rawURL)
	}
	switch kind {
	case store.KindYtdlp, "page":
		return rt.AnalyzeYtdlp(ctx, rawURL, headers, playlist)
	case store.KindHoster:
		if Hosters == nil {
			return nil, errors.New("extracteurs non initialisés")
		}
		res, err := Hosters.Resolve(ctx, rawURL, headers)
		if err != nil {
			return nil, err
		}
		h := map[string]string{}
		for k, v := range headers {
			h[k] = v
		}
		for k, v := range res.Headers {
			h[k] = v
		}
		k := store.KindHTTP
		switch res.Kind {
		case "hls":
			k = store.KindHLS
		case "dash":
			k = store.KindDASH
		}
		info, err := rt.Analyze(ctx, k, res.URL, h, false)
		if err != nil {
			return nil, err
		}
		info.Source = "hoster:" + res.Hoster
		if res.Title != "" {
			info.Title = res.Title
		}
		return info, nil
	case store.KindHLS:
		f := newFetcher(rt.M, headers)
		body, final, err := f.getText(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		base, _ := url.Parse(final)
		pl, err := ParseM3U8(body, base)
		if err != nil {
			return nil, err
		}
		info := &VideoInfo{Source: "hls", Variants: pl.Variants, Live: !pl.Master && !pl.Ended, Duration: pl.Duration}
		for _, r := range pl.Renditions {
			if r.Type == "AUDIO" {
				info.Audio = append(info.Audio, r)
			}
		}
		if pl.SampleAES {
			return nil, ErrDRM
		}
		if pl.Master && len(pl.Variants) > 0 {
			// duration + live flag from the best variant
			if v := BestVariant(pl.Variants, 0); v != nil {
				if b, fu, err := f.getText(ctx, v.URL); err == nil {
					bu, _ := url.Parse(fu)
					if p2, err := ParseM3U8(b, bu); err == nil {
						info.Duration = p2.Duration
						info.Live = !p2.Ended
						if p2.SampleAES {
							return nil, ErrDRM
						}
					}
				}
			}
		}
		return info, nil
	case store.KindDASH:
		f := newFetcher(rt.M, headers)
		body, _, err := f.getText(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		m, err := parseMPD(body)
		if err != nil {
			return nil, err
		}
		reps, _ := DashReps(body)
		for _, p := range m.Periods {
			for _, as := range p.Sets {
				if hasDRM(as.Protection) {
					return nil, ErrDRM
				}
				for _, r := range as.Reps {
					if hasDRM(r.Protection) {
						return nil, ErrDRM
					}
				}
			}
		}
		return &VideoInfo{Source: "dash", Dash: reps, Duration: parseISODuration(m.Duration), Live: strings.EqualFold(m.Type, "dynamic")}, nil
	default:
		info, err := engine.Probe(ctx, rt.M.HTTPClient(), rt.M.EngineConfig(), rawURL, headers)
		if err != nil {
			return nil, err
		}
		return &VideoInfo{Source: "direct", Title: info.FileName, Formats: []Format{{ID: "direct", Ext: strings.TrimPrefix(extOf(info.FileName), "."), Filesize: info.Size}}}, nil
	}
}

func looksLikeMedia(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	for _, e := range []string{".mp4", ".m3u8", ".mpd", ".ts", ".m4s", ".webm", ".mkv", ".mp3", ".m4a"} {
		if strings.HasSuffix(p, e) {
			return true
		}
	}
	return false
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}
