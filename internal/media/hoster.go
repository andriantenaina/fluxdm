package media

import (
	"context"
	"encoding/json"
	"fmt"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/extractors"
	"fluxdm/internal/store"
)

// Hosters is the extractor engine (set by the app).
var Hosters *extractors.Engine

// RunHoster resolves a video host page then downloads the real stream. The
// page is resolved again on every start because hosts issue short-lived links.
func (rt *Runtime) RunHoster(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	if Hosters == nil {
		return fmt.Errorf("extracteurs non initialisés")
	}
	res, err := Hosters.Resolve(ctx, it.URL, it.Headers)
	if err != nil {
		return err
	}
	if it.Title == "" && res.Title != "" {
		rep.Title(res.Title)
	}
	sub := *it
	if sub.Title == "" {
		sub.Title = res.Title
	}
	sub.URL = res.URL
	sub.Headers = map[string]string{}
	for k, v := range it.Headers {
		if k == "Cookie" || k == "User-Agent" {
			sub.Headers[k] = v
		}
	}
	for k, v := range res.Headers {
		sub.Headers[k] = v
	}
	switch res.Kind {
	case "hls":
		sub.Kind = store.KindHLS
		return rt.runFresh(ctx, &sub, rep, rt.resolveHLS, rt.reloadHLS)
	case "dash":
		sub.Kind = store.KindDASH
		return rt.runFresh(ctx, &sub, rep, rt.resolveDASH, func(context.Context, *fetcher, *streamState, *streamJob) (bool, error) { return true, nil })
	default:
		sub.Kind = store.KindHTTP
		if len(sub.State) > 0 {
			var snap engine.Snapshot
			if json.Unmarshal(sub.State, &snap) == nil {
				snap.URL, snap.FinalURL, snap.Headers = res.URL, "", sub.Headers
				sub.State, _ = json.Marshal(snap)
			}
		}
		if sub.FileName == "" && sub.Title != "" {
			sub.FileName = engine.SanitizeFileName(sub.Title) + ".mp4"
		}
		run := m.Runner(store.KindHTTP)
		return run(ctx, m, &sub, rep)
	}
}

type resolveFunc func(ctx context.Context, f *fetcher, it *store.Item) (*streamState, error)

// runFresh resolves tracks anew (fresh signed URLs) and keeps the progress of
// a previous attempt when the segment layout is unchanged.
func (rt *Runtime) runFresh(ctx context.Context, it *store.Item, rep core.Reporter, resolve resolveFunc, reload reloadFunc) error {
	f := newFetcher(rt.M, it.Headers)
	st, err := resolve(ctx, f, it)
	if err != nil {
		return err
	}
	if len(it.State) > 0 {
		var old streamState
		if json.Unmarshal(it.State, &old) == nil && len(old.Tracks) == len(st.Tracks) && len(old.States) == len(st.Tracks) {
			same := true
			for i := range st.Tracks {
				if len(old.Tracks[i].Segs) != len(st.Tracks[i].Segs) || old.Tracks[i].Name != st.Tracks[i].Name {
					same = false
				}
			}
			if same {
				st.States = old.States
			}
		}
	}
	return rt.runStream(ctx, f, it, rep, st, reload)
}
