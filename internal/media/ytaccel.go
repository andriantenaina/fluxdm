package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
)

// errFallback asks RunYtdlp to use the classic yt-dlp downloader.
var errFallback = errors.New("fallback yt-dlp")

type ytFormat struct {
	URL      string            `json:"url"`
	Ext      string            `json:"ext"`
	Protocol string            `json:"protocol"`
	FormatID string            `json:"format_id"`
	VCodec   string            `json:"vcodec"`
	ACodec   string            `json:"acodec"`
	Headers  map[string]string `json:"http_headers"`
	Filesize int64             `json:"filesize"`
}

type ytInfo struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	IsLive    bool       `json:"is_live"`
	Requested []ytFormat `json:"requested_formats"`
	ytFormat
}

type accelState struct {
	Parts []engine.Snapshot `json:"parts"`
}

// runAccelerated downloads the formats selected by yt-dlp with the multi-
// connection engine, then merges them with ffmpeg. Returns errFallback when
// the formats are not plain HTTP files (HLS/DASH fragments, live streams).
func (rt *Runtime) runAccelerated(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	bin, args, cleanup, err := rt.ytdlpBase(ctx, it, it.Headers, it.URL)
	if err != nil {
		return err
	}
	defer cleanup()
	s := m.Settings()
	sel, audio := formatSelector(it, s)
	args = append(args, "-J", "--no-playlist", "-f", sel, "--", it.URL)
	cmd := rt.command(ctx, bin, args)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ytdlpError(stderr.String(), err)
	}
	var info ytInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return errFallback
	}
	parts := info.Requested
	if len(parts) == 0 && info.URL != "" {
		parts = []ytFormat{info.ytFormat}
	}
	if info.IsLive || len(parts) == 0 {
		return errFallback
	}
	for _, p := range parts {
		if p.URL == "" || (p.Protocol != "https" && p.Protocol != "http") {
			return errFallback
		}
	}
	if it.Title == "" && info.Title != "" {
		rep.Title(info.Title)
	}

	ffmpeg, err := rt.Tools.Ensure(ctx, sidecars.FFmpeg)
	if err != nil {
		return errFallback
	}
	tmp := m.TempDir(it.ID)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}

	// resume: keep previous segments when the part has the same size
	var st accelState
	if len(it.State) > 0 {
		_ = json.Unmarshal(it.State, &st)
	}
	cfg := m.EngineConfig()
	host := ""
	if len(parts) > 0 {
		host = hostOfURL(parts[0].URL)
	}
	if n := core.OptInt(it.Options, "connections"); n > 0 {
		cfg.MaxConns = n // fixed by the user for this item
		cfg.AutoConns = false
	} else if n := s.HostConns[host]; n > cfg.MaxConns && cfg.AutoConns {
		cfg.MaxConns = n
	}
	if cfg.FairWeight > 0 {
		// the parts together weigh as much as one download
		cfg.FairWeight = core.PriorityWeight(it.Priority) / float64(max(len(parts), 1))
	}
	// one limiter for the whole item, shared by its parts
	itemLimiter := engine.NewLimiter(int64(core.OptInt(it.Options, "speedLimit")))
	snaps := make([]engine.Snapshot, len(parts))
	for i, p := range parts {
		h := map[string]string{}
		for k, v := range p.Headers {
			h[k] = v
		}
		snap := engine.Snapshot{URL: p.URL, Headers: h, Dir: tmp, FileName: fmt.Sprintf("part%d.%s", i, p.Ext)}
		if i < len(st.Parts) && st.Parts[i].Path != "" && (p.Filesize == 0 || st.Parts[i].Size == p.Filesize) {
			old := st.Parts[i]
			old.URL, old.FinalURL, old.Headers = p.URL, "", h
			snap = old
		}
		snaps[i] = snap
	}

	downloads := make([]*engine.Download, len(parts))
	// Each part saves every ~2 s: combine them into a single persisted state,
	// written at most once per second.
	var mu sync.Mutex
	var lastSave time.Time
	save := func(force bool) {
		mu.Lock()
		defer mu.Unlock()
		if !force && time.Since(lastSave) < time.Second {
			return
		}
		lastSave = time.Now()
		var cur accelState
		var done int64
		for _, d := range downloads {
			if d != nil {
				s := d.Snapshot()
				cur.Parts = append(cur.Parts, s)
				for _, sg := range s.Segments {
					done += sg.Pos - sg.Start
				}
			}
		}
		rep.SaveState(cur, done)
	}
	for i := range parts {
		d := engine.NewDownload(m.HTTPClient(), cfg, snaps[i])
		d.Limiter = itemLimiter
		d.OnSave = func(engine.Snapshot) { save(false) }
		d.OnConns = func(n int) { m.RememberHostConns(host, n) }
		downloads[i] = d
	}
	m.SetLive(it.ID, partsTunable{downloads, itemLimiter})
	defer m.SetLive(it.ID, nil)

	// combined progress
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		var last int64
		lastT := time.Now()
		var speed float64
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				var done, size int64
				conns := 0
				known := true
				var segs []engine.Segment
				for _, d := range downloads {
					conns += d.Conns()
					s := d.Snapshot()
					if s.Size > 0 {
						size += s.Size
					} else {
						known = false
					}
					for _, sg := range s.Segments {
						done += sg.Pos - sg.Start
					}
					if len(downloads) == 1 {
						segs = s.Segments
					}
				}
				if dt := now.Sub(lastT).Seconds(); dt > 0 {
					inst := float64(done-last) / dt
					speed = 0.7*speed + 0.3*inst
				}
				last, lastT = done, now
				if !known {
					size = -1
				}
				rep.Progress(core.Progress{Size: size, Downloaded: done, Speed: speed, Conns: conns, Segments: segs,
					Extra: map[string]any{"parts": len(downloads), "accelerated": true}})
			}
		}
	}()

	errs := make([]error, len(downloads))
	var wg sync.WaitGroup
	for i, d := range downloads {
		wg.Add(1)
		go func(i int, d *engine.Download) {
			defer wg.Done()
			errs[i] = d.Run(ctx)
		}(i, d)
	}
	wg.Wait()
	close(stop)
	save(true)
	for _, e := range errs {
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var he *engine.HTTPError
			if errors.As(e, &he) && (he.StatusCode == 403 || he.StatusCode == 410) {
				return errFallback // YouTube refused the direct download: let yt-dlp handle it
			}
			return e
		}
	}

	// merge / convert
	rep.Status(store.StatusProcessing)
	var inputs []string
	for _, d := range downloads {
		inputs = append(inputs, d.Snapshot().Path)
	}
	outFmt := core.OptString(it.Options, "output")
	if outFmt == "" {
		outFmt = s.HLSOutput
	}
	if audio {
		if outFmt != "m4a" {
			outFmt = "mp3"
		}
	} else if outFmt != "mkv" && outFmt != "webm" {
		outFmt = "mp4"
	}
	name := baseName(it)
	if t := CleanTitle(info.Title); name == "" || strings.HasPrefix(name, "youtube") {
		name = engine.SanitizeFileName(t)
	}
	outTmp := filepath.Join(tmp, "out."+outFmt)
	if err := Remux(context.WithoutCancel(ctx), ffmpeg, rt.Tools, inputs, outTmp, outFmt); err != nil {
		if outFmt == "mp4" { // codec not allowed in MP4 (rare): MKV accepts everything
			outFmt = "mkv"
			outTmp = filepath.Join(tmp, "out.mkv")
			if err2 := Remux(context.WithoutCancel(ctx), ffmpeg, rt.Tools, inputs, outTmp, outFmt); err2 != nil {
				return err
			}
		} else if outFmt == "m4a" {
			outFmt = "mp3"
			outTmp = filepath.Join(tmp, "out.mp3")
			if err2 := Remux(context.WithoutCancel(ctx), ffmpeg, rt.Tools, inputs, outTmp, outFmt); err2 != nil {
				return err
			}
		} else {
			return err
		}
	}
	if err := rt.place(outTmp, it.Dir, name+"."+outFmt, it, rep); err != nil {
		return err
	}
	_ = os.RemoveAll(tmp)
	return nil
}

// partsTunable applies live changes to every part of an accelerated download.
type partsTunable struct {
	parts []*engine.Download
	lim   *engine.Limiter
}

func (p partsTunable) SetMaxConns(n int) {
	for _, d := range p.parts {
		d.SetMaxConns(n)
	}
}

func (p partsTunable) SetSpeedLimit(bps int64) { p.lim.SetRate(bps) }

func (p partsTunable) SetWeight(w float64) {
	for _, d := range p.parts {
		d.SetWeight(w / float64(len(p.parts)))
	}
}

func (p partsTunable) EffectiveLimit() int64 {
	var lim int64
	for _, d := range p.parts {
		if l := d.EffectiveLimit(); l > 0 && (lim == 0 || l < lim) {
			lim = l
		}
	}
	return lim
}

func hostOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
