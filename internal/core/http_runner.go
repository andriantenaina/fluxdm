package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"

	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

// httpRunner downloads a direct HTTP/HTTPS file with the segmented engine.
func httpRunner(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error {
	cfg := m.EngineConfig()
	if n := optInt(it.Options, "connections"); n > 0 {
		cfg.MaxConns = n
	}
	var snap engine.Snapshot
	if len(it.State) > 0 {
		_ = json.Unmarshal(it.State, &snap)
	}
	if snap.URL == "" {
		snap = engine.Snapshot{
			URL:      it.URL,
			Headers:  m.AuthHeaders(it.URL, it.Headers),
			Dir:      it.Dir,
			FileName: it.FileName,
		}
	}
	d := engine.NewDownload(m.HTTPClient(), cfg, snap)
	d.Limiter = engine.NewLimiter(int64(optInt(it.Options, "speedLimit")))
	m.mu.Lock()
	m.live[it.ID] = d
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.live, it.ID)
		m.mu.Unlock()
	}()
	d.OnInfo = func(s engine.Snapshot) {
		rep.Info(s.FileName, s.Path, s.Size)
	}
	d.OnProgress = func(p engine.Progress) {
		rep.Progress(Progress{Size: p.Size, Downloaded: p.Downloaded, Speed: p.Speed, Conns: p.Conns, Segments: p.Segments})
	}
	d.OnSave = func(s engine.Snapshot) {
		var n int64
		for _, sg := range s.Segments {
			n += sg.Pos - sg.Start
		}
		rep.SaveState(s, n)
	}
	err := d.Run(ctx)
	if err == nil {
		s := d.Snapshot()
		rep.Info(s.FileName, s.Path, s.Size)
	}
	return err
}

// AuthHeaders adds stored Basic credentials for the URL host when the request has none.
func (m *Manager) AuthHeaders(rawURL string, h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return out
	}
	if _, has := out["Authorization"]; has {
		return out
	}
	user, pass := "", ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	} else {
		s := m.Settings()
		if c := s.CredFor(u.Hostname()); c != nil {
			user, pass = c.User, c.Password
		}
	}
	if user != "" {
		out["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	}
	return out
}

func optInt(o map[string]any, k string) int {
	switch v := o[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func optString(o map[string]any, k string) string {
	s, _ := o[k].(string)
	return s
}

func optBool(o map[string]any, k string) bool {
	b, _ := o[k].(bool)
	return b
}

// Exported option helpers for runners in other packages.
var (
	OptInt    = optInt
	OptString = optString
	OptBool   = optBool
)
