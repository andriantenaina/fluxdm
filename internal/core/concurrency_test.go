package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"fluxdm/internal/store"
)

func TestPriorityOrderAndReorder(t *testing.T) {
	m := newMgr(t)
	var mu sync.Mutex
	var started []string
	release := make(chan struct{})
	m.Register("fake", func(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error {
		mu.Lock()
		started = append(started, it.ID)
		mu.Unlock()
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	s := m.Settings()
	s.MaxActive = 1
	s.AutoActive = false
	_ = m.SetSettings(s)
	// a blocker keeps the only slot busy while we arrange the queue
	blocker, _ := m.Add(AddRequest{URL: "https://a.example/0", Kind: "fake", Start: true})
	wait(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(started) == 1 })
	var ids []string
	for i := 0; i < 4; i++ {
		it, err := m.Add(AddRequest{URL: "https://a.example/" + string(rune('a'+i)), Kind: "fake", Start: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
		time.Sleep(2 * time.Millisecond) // distinct creation dates
	}
	// expected run order: ids[3] (high priority), then ids[2] moved before ids[0], ids[0], ids[1]
	m.SetPriority([]string{ids[3]}, 1)
	if err := m.Reorder(ids[2], ids[0]); err != nil {
		t.Fatal(err)
	}
	if it, _ := m.Get(ids[2]); it.Order <= 0 {
		t.Errorf("order non attribué : %v", it.Order)
	}
	want := []string{blocker.ID, ids[3], ids[2], ids[0], ids[1]}
	for len(want) > 0 {
		release <- struct{}{}
		time.Sleep(30 * time.Millisecond)
		want = want[1:]
	}
	wait(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(started) == 5 })
	mu.Lock()
	got := append([]string(nil), started...)
	mu.Unlock()
	exp := []string{blocker.ID, ids[3], ids[2], ids[0], ids[1]}
	for i := range exp {
		if got[i] != exp[i] {
			t.Fatalf("ordre de démarrage %v, attendu %v", got, exp)
		}
	}
	close(release)
}

func TestReorderRenumber(t *testing.T) {
	m := newMgr(t)
	m.Register("fake", func(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error { return nil })
	var ids []string
	for i := 0; i < 3; i++ {
		it, _ := m.Add(AddRequest{URL: "https://b.example/" + string(rune('a'+i)), Kind: "fake"})
		ids = append(ids, it.ID)
		time.Sleep(2 * time.Millisecond) // distinct creation dates
	}
	// moving repeatedly between the same neighbours eventually forces a renumbering
	for i := 0; i < 80; i++ {
		mv, before := ids[2], ids[1]
		if i%2 == 1 {
			mv, before = ids[1], ids[2]
		}
		if err := m.Reorder(mv, before); err != nil {
			t.Fatal(err)
		}
	}
	order := func(id string) float64 { it, _ := m.Get(id); return it.SortKey() }
	// last move (i = 79): ids[1] before ids[2]
	if !(order(ids[0]) < order(ids[1]) && order(ids[1]) < order(ids[2])) {
		t.Errorf("ordre final incohérent : %v %v %v", order(ids[0]), order(ids[1]), order(ids[2]))
	}
	if err := m.Reorder(ids[0], ""); err != nil {
		t.Fatal(err)
	}
	if order(ids[0]) <= order(ids[2]) {
		t.Error("déplacement en fin de file non appliqué")
	}
}

func TestCategoryKeysAndTotals(t *testing.T) {
	s := Settings{Categories: []Category{{Name: "Vidéos"}, {Name: "Music"}, {Name: "Mes trucs"}}}
	s.normalize()
	if s.Categories[0].Key != "video" || s.Categories[1].Key != "audio" || s.Categories[2].Key != "" {
		t.Errorf("migration des clés : %+v", s.Categories)
	}
	if s.MaxConnsPerHost != 16 || !s.AutoActive || !s.FairShare {
		t.Errorf("valeurs par défaut : %+v", s)
	}
	if s.CategoryName("video") != "Vidéos" || s.CategoryName("torrent") != "Torrents" {
		t.Error("CategoryName")
	}
	m := newMgr(t)
	m.Register("fake", func(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error {
		rep.Progress(Progress{Size: 1000, Downloaded: 400, Speed: 100})
		<-ctx.Done()
		return ctx.Err()
	})
	it, _ := m.Add(AddRequest{URL: "https://c.example/x", Kind: "fake", Start: true, Size: 1000})
	wait(t, func() bool { return m.LatestProgress(it.ID) != nil })
	tot := m.Totals()
	if tot.Active != 1 || tot.Remaining != 600 || tot.ETA != 6 {
		t.Errorf("totaux : %+v", tot)
	}
	p := m.LatestProgress(it.ID)
	if p.ETA != ETACalculating || p.StartedAt == 0 {
		t.Errorf("progression : %+v", p)
	}
}
