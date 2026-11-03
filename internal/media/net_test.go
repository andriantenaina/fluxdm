package media

import (
	"os"
	"testing"

	"fluxdm/internal/core"
	"fluxdm/internal/store"
)

// Real network test (opt-in): FLUXDM_NET=1 go test ./internal/media -run RealHLS -v
func TestRealHLS(t *testing.T) {
	if os.Getenv("FLUXDM_NET") == "" {
		t.Skip("FLUXDM_NET=1 pour activer les tests réseau")
	}
	m, _ := newTestManager(t)
	it, err := m.Add(core.AddRequest{
		URL:     "https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8",
		Kind:    store.KindHLS,
		Title:   "Big Buck Bunny",
		Options: map[string]any{"output": "ts", "maxHeight": float64(240)},
		Start:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := waitDoneLong(t, m, it.ID)
	if got.Status != store.StatusCompleted {
		t.Fatalf("échec : %s", got.Error)
	}
	fi, err := os.Stat(got.Path)
	if err != nil || fi.Size() < 1<<20 {
		t.Fatalf("fichier final invalide : %v %v", fi, err)
	}
	b := make([]byte, 189)
	f, _ := os.Open(got.Path)
	f.Read(b)
	f.Close()
	if b[0] != 0x47 || b[188] != 0x47 {
		t.Error("le fichier n'est pas un flux MPEG-TS valide")
	}
	t.Logf("%s : %.1f Mo", got.Path, float64(fi.Size())/(1<<20))
}
