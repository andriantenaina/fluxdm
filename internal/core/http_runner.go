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
	host := hostOf(it.URL)
	if n := optInt(it.Options, "connections"); n > 0 {
		cfg.MaxConns = n // fixed by the user for this file
		cfg.AutoConns = false
	} else if n := m.Settings().HostConns[host]; n > cfg.MaxConns && cfg.AutoConns {
		cfg.MaxConns = n // start where auto-tuning usually ends on this host
	}
	if cfg.FairWeight > 0 {
		cfg.FairWeight = PriorityWeight(it.Priority)
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
	if ms, ok := it.Options["mirrors"].([]any); ok {
		snap.Mirrors = snap.Mirrors[:0]
		for _, v := range ms {
			if s, ok := v.(string); ok {
				snap.Mirrors = append(snap.Mirrors, s)
			}
		}
	}
	d := engine.NewDownload(m.HTTPClient(), cfg, snap)
	d.OnConns = func(n int) { m.RememberHostConns(host, n) }
	d.Limiter = engine.NewLimiter(int64(optInt(it.Options, "speedLimit")))
	m.SetLive(it.ID, d)
	defer m.SetLive(it.ID, nil)
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

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// RememberHostConns stores a smoothed connection count settled by
// auto-tuning (an average rather than the maximum ever seen, so one lucky
// download does not make every later one start with too many connections).
func (m *Manager) RememberHostConns(host string, n int) {
	if host == "" || n <= 0 {
		return
	}
	m.mu.Lock()
	if m.settings.HostConns == nil {
		m.settings.HostConns = map[string]int{}
	}
	v := n
	if old := m.settings.HostConns[host]; old > 0 {
		// move 30 % of the way towards n, at least one step
		step := max(1, (abs(n-old)*3+5)/10)
		switch {
		case n > old:
			v = old + min(step, n-old)
		case n < old:
			v = old - min(step, old-n)
		default:
			v = old
		}
	}
	if m.settings.HostConns[host] == v {
		m.mu.Unlock()
		return
	}
	m.settings.HostConns[host] = v
	if len(m.settings.HostConns) > 300 { // keep the map small
		for k := range m.settings.HostConns {
			delete(m.settings.HostConns, k)
			break
		}
	}
	s := m.settings
	m.mu.Unlock()
	_ = m.Store.SetSetting("settings", s)
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
