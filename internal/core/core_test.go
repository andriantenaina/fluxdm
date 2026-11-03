package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fluxdm/internal/store"
)

func newMgr(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	s := m.Settings()
	s.DownloadDir = filepath.Join(dir, "dl")
	if err := m.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	return m
}

func wait(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("délai dépassé")
}

func TestHTTPDownloadAndCategory(t *testing.T) {
	data := make([]byte, 2<<20)
	rand.Read(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.zip", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()
	m := newMgr(t)
	var completed atomic.Bool
	m.Subscribe(func(ev Event) {
		if ev.Type == "completed" {
			completed.Store(true)
		}
	})
	it, err := m.Add(AddRequest{URL: srv.URL + "/files/archive.zip", Start: true})
	if err != nil {
		t.Fatal(err)
	}
	if it.Category != "Compressés" || filepath.Base(it.Dir) != "Compressés" {
		t.Errorf("catégorie=%q dossier=%q", it.Category, it.Dir)
	}
	wait(t, completed.Load)
	got, _ := m.Get(it.ID)
	if got.Status != store.StatusCompleted || got.Size != int64(len(data)) {
		t.Fatalf("item = %+v", got)
	}
	b, _ := os.ReadFile(got.Path)
	if !bytes.Equal(b, data) {
		t.Fatal("contenu différent")
	}
	// persisted across restart
	m2, err := NewManager(m.DataDir + "")
	if err == nil {
		defer m2.Store.Close()
		if it2, err := m2.Get(it.ID); err != nil || it2.Status != store.StatusCompleted {
			t.Errorf("non persisté : %v %v", it2, err)
		}
	}
}

func TestQueueLimits(t *testing.T) {
	m := newMgr(t)
	var running, peak int32
	release := make(chan struct{})
	m.Register("fake", func(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error {
		n := atomic.AddInt32(&running, 1)
		defer atomic.AddInt32(&running, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	s := m.Settings()
	s.MaxActive = 2
	_ = m.SetSettings(s)
	var ids []string
	for i := 0; i < 5; i++ {
		it, err := m.Add(AddRequest{URL: "https://example.com/f" + string(rune('a'+i)), Kind: "fake", Start: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	wait(t, func() bool { return atomic.LoadInt32(&running) == 2 })
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&peak) != 2 {
		t.Fatalf("pic = %d, attendu 2 (MaxActive)", peak)
	}
	// pause one queued item, then release everything
	_ = m.Pause(ids[4])
	close(release)
	wait(t, func() bool {
		done := 0
		for _, id := range ids[:4] {
			if it, _ := m.Get(id); it.Status == store.StatusCompleted {
				done++
			}
		}
		return done == 4
	})
	if it, _ := m.Get(ids[4]); it.Status != store.StatusPaused {
		t.Errorf("l'élément mis en pause a démarré : %s", it.Status)
	}
	// custom queue not running: items stay queued until StartQueue
	_ = m.SaveQueue(QueueState{Name: "nuit", MaxActive: 1})
	it, _ := m.Add(AddRequest{URL: "https://example.com/n", Kind: "fake", Queue: "nuit"})
	_ = m.Enqueue(it.ID)
	time.Sleep(100 * time.Millisecond)
	if g, _ := m.Get(it.ID); g.Status != store.StatusQueued {
		t.Errorf("file arrêtée mais statut %s", g.Status)
	}
	m.StartQueue("nuit")
	wait(t, func() bool { g, _ := m.Get(it.ID); return g.Status == store.StatusCompleted })
	if !m.QueueIdle("nuit") {
		t.Error("la file devrait être inactive")
	}
}

func TestPauseResumeAndRemove(t *testing.T) {
	data := make([]byte, 4<<20)
	rand.Read(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		http.ServeContent(w, r, "v.mp4", time.Unix(1, 0), bytes.NewReader(data))
	}))
	defer srv.Close()
	m := newMgr(t)
	s := m.Settings()
	s.SpeedLimit = 1 << 20 // keep it slow enough to pause
	_ = m.SetSettings(s)
	it, _ := m.Add(AddRequest{URL: srv.URL + "/v.mp4", Start: true})
	wait(t, func() bool { g, _ := m.Get(it.ID); return g.Downloaded > 0 })
	if err := m.Pause(it.ID); err != nil {
		t.Fatal(err)
	}
	g, _ := m.Get(it.ID)
	if g.Status != store.StatusPaused || len(g.State) == 0 {
		t.Fatalf("pause : statut=%s état=%d", g.Status, len(g.State))
	}
	s.SpeedLimit = 0
	_ = m.SetSettings(s)
	if err := m.Start(it.ID); err != nil {
		t.Fatal(err)
	}
	wait(t, func() bool { g, _ := m.Get(it.ID); return g.Status == store.StatusCompleted })
	g, _ = m.Get(it.ID)
	b, _ := os.ReadFile(g.Path)
	if !bytes.Equal(b, data) {
		t.Fatal("contenu différent après reprise")
	}
	if err := m.Remove(it.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(g.Path); err == nil {
		t.Error("fichier non supprimé")
	}
}

func TestDetectKind(t *testing.T) {
	cases := map[string]string{
		"magnet:?xt=urn:btih:abc":       store.KindTorrent,
		"https://x/a.torrent":           store.KindTorrent,
		"https://x/live/index.m3u8?a=b": store.KindHLS,
		"https://x/v/manifest.mpd":      store.KindDASH,
		"ftp://x/file.iso":              store.KindFTP,
		"https://x/file.zip":            store.KindHTTP,
	}
	for u, want := range cases {
		if got := DetectKind(u); got != want {
			t.Errorf("DetectKind(%s) = %s, attendu %s", u, got, want)
		}
	}
}

func TestCredentials(t *testing.T) {
	s := DefaultSettings()
	s.Credentials = []Cred{{Host: "*.example.com", User: "u", Password: "p"}}
	if c := s.CredFor("files.example.com"); c == nil || c.User != "u" {
		t.Error("joker *.example.com non appliqué")
	}
	if s.CredFor("example.org") != nil {
		t.Error("faux positif")
	}
}
