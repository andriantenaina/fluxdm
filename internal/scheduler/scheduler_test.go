package scheduler

import (
	"testing"
	"time"

	"fluxdm/internal/core"
)

func TestDaysAndFiredOnce(t *testing.T) {
	if !dayAllowed(nil, time.Monday) || !dayAllowed([]int{1, 3}, time.Wednesday) || dayAllowed([]int{0}, time.Friday) {
		t.Error("dayAllowed incorrect")
	}
	m, err := core.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	s := New(m)
	if !s.firedOnce("q/start", "2026-11-03 02:00") || s.firedOnce("q/start", "2026-11-03 02:00") {
		t.Error("un horaire ne doit se déclencher qu'une fois par minute")
	}
}

func TestQuota(t *testing.T) {
	m, err := core.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	s := New(m)
	if err := s.Save(Config{Quota: Quota{Enabled: true, MB: 1, Hours: 1}}); err != nil {
		t.Fatal(err)
	}
	s.onEvent(core.Event{Type: "progress", ID: "a", Progress: &core.Progress{Downloaded: 0}})
	s.onEvent(core.Event{Type: "progress", ID: "a", Progress: &core.Progress{Downloaded: 2 << 20}})
	s.checkQuota(s.Config().Quota)
	if !s.Status().QuotaPaused {
		t.Fatal("le quota dépassé doit suspendre les téléchargements")
	}
	if used := s.Status().QuotaUsedMB; used < 1.9 {
		t.Errorf("utilisé = %.2f Mo", used)
	}
	s.Save(Config{Quota: Quota{Enabled: false}})
	s.checkQuota(s.Config().Quota)
	if s.Status().QuotaPaused {
		t.Error("désactiver le quota doit reprendre")
	}
}
