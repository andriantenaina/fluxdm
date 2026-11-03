// Package torrent downloads magnet links and .torrent files (anacrolix/torrent).
package torrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"fluxdm/internal/core"
	"fluxdm/internal/store"
)

// FileInfo describes a file inside a torrent (add dialog / file selection).
type FileInfo struct {
	Index int    `json:"index"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
}

// Info is returned for a .torrent file before adding it.
type Info struct {
	Name     string     `json:"name"`
	InfoHash string     `json:"infoHash"`
	Size     int64      `json:"size"`
	Files    []FileInfo `json:"files"`
}

// Service owns the shared BitTorrent client.
type Service struct {
	m       *core.Manager
	dataDir string

	mu      sync.Mutex
	client  *atorrent.Client
	dlLimit *rate.Limiter
	seeding map[string]*atorrent.Torrent
}

// New creates the service; the client starts lazily on first use.
func New(m *core.Manager, dataDir string) *Service {
	return &Service{m: m, dataDir: dataDir, seeding: map[string]*atorrent.Torrent{}}
}

func (s *Service) get() (*atorrent.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.applyLimitLocked()
		return s.client, nil
	}
	set := s.m.Settings()
	cfg := atorrent.NewDefaultClientConfig()
	cfg.DataDir = set.DownloadDir
	cfg.Seed = set.TorrentSeed
	cfg.NoUpload = false
	cfg.ListenPort = set.TorrentPort // 0 = random
	cfg.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: set.DownloadDir})
	s.dlLimit = rate.NewLimiter(rate.Inf, 1<<20)
	cfg.DownloadRateLimiter = s.dlLimit
	cl, err := atorrent.NewClient(cfg)
	if err != nil {
		// port busy: retry with a random port
		cfg.ListenPort = 0
		if cl, err = atorrent.NewClient(cfg); err != nil {
			return nil, fmt.Errorf("client BitTorrent : %w", err)
		}
	}
	s.client = cl
	s.applyLimitLocked()
	return cl, nil
}

func (s *Service) applyLimitLocked() {
	if s.dlLimit == nil {
		return
	}
	if r := s.m.Limiter().Rate(); r > 0 {
		s.dlLimit.SetLimit(rate.Limit(r))
		s.dlLimit.SetBurst(int(min(r, 1<<20)))
	} else {
		s.dlLimit.SetLimit(rate.Inf)
	}
}

// Close stops the client.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// loadMetaInfo reads a .torrent from a local path or URL.
func (s *Service) loadMetaInfo(ctx context.Context, src string, headers map[string]string) (*metainfo.MetaInfo, error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := s.m.HTTPClient().Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %s", resp.Status)
		}
		return metainfo.Load(io.LimitReader(resp.Body, 32<<20))
	}
	return metainfo.LoadFromFile(src)
}

// Inspect returns the content of a .torrent (no network for local files).
func (s *Service) Inspect(ctx context.Context, src string, headers map[string]string) (*Info, error) {
	if strings.HasPrefix(strings.ToLower(src), "magnet:") {
		spec, err := atorrent.TorrentSpecFromMagnetUri(src)
		if err != nil {
			return nil, err
		}
		return &Info{Name: spec.DisplayName, InfoHash: spec.InfoHash.HexString(), Size: -1}, nil
	}
	mi, err := s.loadMetaInfo(ctx, src, headers)
	if err != nil {
		return nil, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, err
	}
	out := &Info{Name: info.BestName(), InfoHash: mi.HashInfoBytes().HexString(), Size: info.TotalLength()}
	for i, f := range info.UpvertedFiles() {
		p := strings.Join(f.BestPath(), "/")
		if p == "" {
			p = info.BestName()
		}
		out.Files = append(out.Files, FileInfo{Index: i, Path: p, Size: f.Length})
	}
	return out, nil
}

// Runner is the core.Runner for store.KindTorrent.
func (s *Service) Runner(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	cl, err := s.get()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(it.Dir, 0o755); err != nil {
		return err
	}
	var spec *atorrent.TorrentSpec
	if strings.HasPrefix(strings.ToLower(it.URL), "magnet:") {
		spec, err = atorrent.TorrentSpecFromMagnetUri(it.URL)
	} else {
		var mi *metainfo.MetaInfo
		if mi, err = s.loadMetaInfo(ctx, it.URL, it.Headers); err == nil {
			spec, err = atorrent.TorrentSpecFromMetaInfoErr(mi)
		}
	}
	if err != nil {
		return fmt.Errorf("torrent invalide : %w", err)
	}
	spec.Storage = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: it.Dir})

	s.mu.Lock()
	if old := s.seeding[it.ID]; old != nil {
		old.Drop()
		delete(s.seeding, it.ID)
	}
	s.mu.Unlock()

	t, _, err := cl.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			t.Drop()
		}
	}()

	rep.Progress(core.Progress{Size: -1, Extra: map[string]any{"phase": "metadata"}})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.GotInfo():
	case <-time.After(10 * time.Minute):
		return errors.New("impossible d'obtenir les métadonnées du torrent (aucun pair)")
	}

	// file selection
	files := t.Files()
	var selected map[int]bool
	if raw, ok := it.Options["files"].([]any); ok && len(raw) > 0 {
		selected = map[int]bool{}
		for _, v := range raw {
			if f, ok := v.(float64); ok {
				selected[int(f)] = true
			}
		}
	}
	var total int64
	var fileInfos []FileInfo
	for i, f := range files {
		fileInfos = append(fileInfos, FileInfo{Index: i, Path: f.DisplayPath(), Size: f.Length()})
		if selected == nil || selected[i] {
			f.Download()
			total += f.Length()
		} else {
			f.SetPriority(atorrent.PiecePriorityNone)
		}
	}
	name := t.Name()
	rep.Info(name, filepath.Join(it.Dir, name), total)

	completed := func() int64 {
		var n int64
		for i, f := range files {
			if selected == nil || selected[i] {
				n += f.BytesCompleted()
			}
		}
		return n
	}

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	last := completed()
	lastT := time.Now()
	var speed float64
	lastSave := time.Now()
	for {
		select {
		case <-ctx.Done():
			rep.SaveState(map[string]any{"infoHash": t.InfoHash().HexString()}, last)
			return ctx.Err()
		case now := <-tick.C:
			done := completed()
			dt := now.Sub(lastT).Seconds()
			if dt > 0 {
				inst := float64(done-last) / dt
				speed = 0.7*speed + 0.3*inst
			}
			last, lastT = done, now
			st := t.Stats()
			rep.Progress(core.Progress{Size: total, Downloaded: done, Speed: speed, Conns: st.ActivePeers,
				Extra: map[string]any{"peers": st.ActivePeers, "seeds": st.ConnectedSeeders, "totalPeers": st.TotalPeers, "files": fileInfos}})
			if time.Since(lastSave) > 5*time.Second {
				lastSave = time.Now()
				rep.SaveState(map[string]any{"infoHash": t.InfoHash().HexString()}, done)
			}
			if total > 0 && done >= total {
				if m.Settings().TorrentSeed {
					keep = true
					s.mu.Lock()
					s.seeding[it.ID] = t
					s.mu.Unlock()
				}
				return nil
			}
		}
	}
}

// StopSeeding drops a seeding torrent (when the item is removed).
func (s *Service) StopSeeding(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.seeding[id]; t != nil {
		t.Drop()
		delete(s.seeding, id)
	}
}
