package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/store"
)

// ErrDRM is returned for streams protected by DRM (not supported, by design).
var ErrDRM = errors.New("ce flux est protégé par DRM (Widevine/FairPlay/PlayReady) : téléchargement impossible")

type streamState struct {
	Tracks []Track        `json:"tracks"`
	States []trackState   `json:"states"`
	Live   bool           `json:"live"`
	Src    []string       `json:"src"` // media playlist URL per track (live reload)
	Extra  map[string]any `json:"extra,omitempty"`
}

// maxHeightFor converts the quality preference into a max height (0 = best).
func maxHeightFor(it *store.Item, s core.Settings) (int, bool) {
	q := core.OptString(it.Options, "quality")
	if q == "" {
		q = s.VideoQuality
	}
	if core.OptBool(it.Options, "audioOnly") || q == "audio" {
		return 0, true
	}
	if h := core.OptInt(it.Options, "maxHeight"); h > 0 {
		return h, false
	}
	h, _ := strconv.Atoi(strings.TrimSuffix(q, "p"))
	return h, false
}

// resolveHLS turns the item URL (master or media playlist) into tracks.
func (rt *Runtime) resolveHLS(ctx context.Context, f *fetcher, it *store.Item) (*streamState, error) {
	body, final, err := f.getText(ctx, it.URL)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(final)
	pl, err := ParseM3U8(body, base)
	if err != nil {
		return nil, err
	}
	st := &streamState{}
	videoURL, audioURL := final, ""
	if pl.Master {
		maxH, audioOnly := maxHeightFor(it, rt.M.Settings())
		var chosen *Variant
		if want := core.OptString(it.Options, "variantUrl"); want != "" {
			for i := range pl.Variants {
				if pl.Variants[i].URL == want {
					chosen = &pl.Variants[i]
				}
			}
		}
		if chosen == nil && audioOnly {
			// prefer an audio rendition, else the lowest variant
			for _, r := range pl.Renditions {
				if r.Type == "AUDIO" && r.URI != "" {
					audioURL = r.URI
					if r.Default {
						break
					}
				}
			}
			if audioURL != "" {
				videoURL = ""
			} else {
				chosen = BestVariant(pl.Variants, 1)
			}
		}
		if chosen == nil && videoURL != "" {
			chosen = BestVariant(pl.Variants, maxH)
		}
		if chosen != nil {
			videoURL = chosen.URL
			if chosen.AudioGroup != "" && audioURL == "" {
				lang := core.OptString(it.Options, "audioLang")
				for _, r := range pl.Renditions {
					if r.Type == "AUDIO" && r.GroupID == chosen.AudioGroup && r.URI != "" {
						if audioURL == "" || r.Default || (lang != "" && strings.HasPrefix(r.Language, lang)) {
							audioURL = r.URI
						}
						if lang != "" && strings.HasPrefix(r.Language, lang) {
							break
						}
					}
				}
			}
		}
		if videoURL == "" && audioURL == "" {
			return nil, errors.New("aucune variante exploitable dans la playlist")
		}
	}

	add := func(name, src string, pre *Playlist) error {
		p := pre
		if p == nil {
			b, fu, err := f.getText(ctx, src)
			if err != nil {
				return err
			}
			bu, _ := url.Parse(fu)
			if p, err = ParseM3U8(b, bu); err != nil {
				return err
			}
			if p.Master {
				return errors.New("playlist imbriquée inattendue")
			}
		}
		if p.SampleAES {
			return ErrDRM
		}
		t := Track{Name: name, Init: p.Init, Segs: p.Segments}
		t.FMP4 = p.Init != nil || (len(p.Segments) > 0 && isFMP4URL(p.Segments[0].URL))
		st.Tracks = append(st.Tracks, t)
		st.Src = append(st.Src, src)
		if !p.Ended {
			st.Live = true
		}
		return nil
	}
	if !pl.Master {
		if err := add("main", final, pl); err != nil {
			return nil, err
		}
		return st, nil
	}
	if videoURL != "" {
		name := "main"
		if audioURL != "" {
			name = "video"
		}
		if err := add(name, videoURL, nil); err != nil {
			return nil, err
		}
	}
	if audioURL != "" {
		if err := add("audio", audioURL, nil); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func isFMP4URL(u string) bool {
	p := strings.ToLower(u)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".m4s") || strings.HasSuffix(p, ".mp4") || strings.HasSuffix(p, ".m4v") || strings.HasSuffix(p, ".m4a") || strings.HasSuffix(p, ".cmfv")
}

// RunHLS is the runner for store.KindHLS.
func (rt *Runtime) RunHLS(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	f := newFetcher(m, it.Headers)
	var st *streamState
	if len(it.State) > 0 {
		var s streamState
		if json.Unmarshal(it.State, &s) == nil && len(s.Tracks) > 0 {
			st = &s
		}
	}
	if st == nil {
		var err error
		if st, err = rt.resolveHLS(ctx, f, it); err != nil {
			return err
		}
	}
	return rt.runStream(ctx, f, it, rep, st, rt.reloadHLS)
}

// reloadHLS appends new segments of a live playlist; returns true when ended.
func (rt *Runtime) reloadHLS(ctx context.Context, f *fetcher, st *streamState, j *streamJob) (bool, error) {
	ended := true
	for ti, src := range st.Src {
		b, fu, err := f.getText(ctx, src)
		if err != nil {
			return false, err
		}
		bu, _ := url.Parse(fu)
		p, err := ParseM3U8(b, bu)
		if err != nil {
			return false, err
		}
		if !p.Ended {
			ended = false
		}
		t := &j.tracks[ti]
		var last int64 = -1
		if n := len(t.Segs); n > 0 {
			last = t.Segs[n-1].Seq
		}
		j.mu.Lock()
		for _, s := range p.Segments {
			if s.Seq > last {
				t.Segs = append(t.Segs, s)
				j.states[ti].Done = append(j.states[ti].Done, 0)
				j.total++
			}
		}
		j.mu.Unlock()
	}
	return ended, nil
}

type reloadFunc func(ctx context.Context, f *fetcher, st *streamState, j *streamJob) (bool, error)

// runStream downloads tracks (VOD or live) then finalizes the output file.
func (rt *Runtime) runStream(ctx context.Context, f *fetcher, it *store.Item, rep core.Reporter, st *streamState, reload reloadFunc) error {
	s := rt.M.Settings()
	workers := core.OptInt(it.Options, "concurrency")
	if workers <= 0 {
		workers = s.HLSConcurrency
	}
	tmp := rt.M.TempDir(it.ID)
	j := &streamJob{f: f, tmp: tmp, tracks: st.Tracks, states: st.States, workers: workers}
	j.save = func() {
		j.mu.Lock()
		st.Tracks = j.tracks
		st.States = make([]trackState, len(j.states))
		for i := range j.states {
			st.States[i] = trackState{Name: j.states[i].Name, Done: append([]int64(nil), j.states[i].Done...)}
		}
		j.mu.Unlock()
		done, _ := j.estimate()
		rep.SaveState(st, done)
	}
	rep.Info(baseName(it)+"."+rt.outputFormat(it), "", 0)

	// progress reporter
	stopProg := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopProg) }) }
	defer stop()
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		var lastBytes int64
		lastT := time.Now()
		var speed float64
		for {
			select {
			case <-stopProg:
				return
			case now := <-t.C:
				done, total := j.estimate()
				dt := now.Sub(lastT).Seconds()
				if dt > 0 {
					inst := float64(done-lastBytes) / dt
					if speed == 0 {
						speed = inst
					} else {
						speed = 0.75*speed + 0.25*inst
					}
				}
				lastBytes, lastT = done, now
				j.mu.Lock()
				segTotal := j.total
				j.mu.Unlock()
				size := total
				if st.Live {
					size = -1
				}
				rep.Progress(core.Progress{Size: size, Downloaded: done, Speed: speed, Conns: workers,
					Extra: map[string]any{"segments": j.segsDone.Load(), "segmentsTotal": segTotal, "live": st.Live, "tracks": len(j.tracks)}})
			}
		}
	}()

	if st.Live {
		target := 4 * time.Second
		for {
			err := j.run(ctx)
			if ctx.Err() != nil {
				break // user stopped the recording: finalize what we have
			}
			if err != nil {
				return err
			}
			ended, err := reload(ctx, f, st, j)
			if err != nil && ctx.Err() == nil {
				return err
			}
			if ended {
				break
			}
			if sleep(ctx, target) != nil {
				break
			}
			j.bytes.Store(0)
			j.segsDone.Store(0)
			j.total = 0
		}
	} else {
		if err := j.run(ctx); err != nil {
			return err
		}
	}
	stop()
	if err := rt.finalize(context.WithoutCancel(ctx), j, it, rep); err != nil {
		return err
	}
	_ = os.RemoveAll(tmp)
	return nil
}
