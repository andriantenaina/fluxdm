package core

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// feed simulates a download sampled every 250 ms; rate(t) gives bytes/s at t.
// It returns the displayed ETA at each tick.
type etaSim struct {
	e    *etaEstimator
	now  time.Time
	done float64
	size int64
}

func newSim(size int64) *etaSim {
	t0 := time.Unix(1_700_000_000, 0)
	return &etaSim{e: newETA(t0, 0), now: t0, size: size}
}

func (s *etaSim) step(rate float64, limit int64) etaResult {
	s.now = s.now.Add(250 * time.Millisecond)
	s.done += rate / 4
	if s.done > float64(s.size) {
		s.done = float64(s.size)
	}
	return s.e.update(s.now, int64(s.done), s.size, limit)
}

func TestETACalculatingAndUnknown(t *testing.T) {
	s := newSim(100 << 20)
	r := s.step(10<<20, 0)
	if r.ETA != ETACalculating {
		t.Fatalf("ETA au démarrage = %v, attendu -2", r.ETA)
	}
	for i := 0; i < 11; i++ { // 3 s at 10 MiB/s: past the warm-up
		r = s.step(10<<20, 0)
	}
	if r.ETA < 0 {
		t.Fatalf("ETA toujours en calcul après 3 s : %v", r.ETA)
	}
	u := newETA(time.Unix(0, 0), 0).update(time.Unix(5, 0), 5<<20, -1, 0)
	if u.ETA != ETAUnknown {
		t.Errorf("taille inconnue : ETA = %v, attendu -1", u.ETA)
	}
	// slow start: under 1 MiB it stays "calculating"
	s2 := newSim(10 << 20)
	for i := 0; i < 40; i++ {
		r = s2.step(20<<10, 0) // 20 KiB/s, 200 KiB after 10 s
	}
	if r.ETA != ETACalculating {
		t.Errorf("moins de 1 Mio : ETA = %v, attendu -2", r.ETA)
	}
}

func TestETAStableRate(t *testing.T) {
	const rate = 5 << 20
	size := int64(300 << 20) // 60 s
	s := newSim(size)
	var prev float64 = -10
	for i := 0; i < 200; i++ { // 50 s
		r := s.step(rate, 0)
		if r.ETA < 0 {
			continue
		}
		want := (float64(size) - s.done) / rate
		if math.Abs(r.ETA-want) > 0.05*want+1 {
			t.Fatalf("t=%.2fs ETA=%.1f attendu %.1f", float64(i+1)/4, r.ETA, want)
		}
		if prev > 0 && r.ETA > prev+0.01 {
			t.Fatalf("l'ETA remonte à débit constant : %.2f -> %.2f", prev, r.ETA)
		}
		prev = r.ETA
		if i > 60 && r.Confidence < 0.8 {
			t.Errorf("confiance trop basse à débit stable : %.2f", r.Confidence)
		}
	}
}

func TestETABursts(t *testing.T) {
	// Segment/fragment downloads report in bursts: each second, one tick
	// carries 4x the mean speed and the three others nothing.
	const mean = 4 << 20
	size := int64(400 << 20)
	s := newSim(size)
	rng := rand.New(rand.NewPCG(1, 2))
	var etas []float64
	var conf float64
	burstAt := 0
	for i := 0; i < 160; i++ { // 40 s
		if i%4 == 0 {
			burstAt = rng.IntN(4) // one burst per second, at a random tick
		}
		rate := 0.0
		if i%4 == burstAt {
			rate = 4 * mean
		}
		r := s.step(rate, 0)
		if i > 40 && r.ETA > 0 {
			want := (float64(size) - s.done) / mean
			if math.Abs(r.ETA-want) > 0.15*want {
				t.Fatalf("rafales : ETA=%.1f attendu ≈%.1f", r.ETA, want)
			}
			etas = append(etas, r.ETA)
			conf = r.Confidence
		}
	}
	// displayed value is smooth: no jump above 15 %/s + 1 s
	for i := 1; i < len(etas); i++ {
		if d := math.Abs(etas[i] - etas[i-1]); d > 0.15*etas[i-1]/4+0.26 {
			t.Fatalf("saut de l'ETA affiché : %.2f -> %.2f", etas[i-1], etas[i])
		}
	}
	if conf > 0.5 {
		t.Errorf("confiance %.2f trop haute pour un débit en rafales", conf)
	}
}

func TestETAStepChange(t *testing.T) {
	size := int64(1 << 30)
	s := newSim(size)
	for i := 0; i < 120; i++ { // 30 s at 10 MiB/s
		s.step(10<<20, 0)
	}
	var r etaResult
	for i := 0; i < 80; i++ { // then 20 s at 2 MiB/s
		r = s.step(2<<20, 0)
	}
	want := (float64(size) - s.done) / (2 << 20)
	if math.Abs(r.ETA-want) > 0.1*want {
		t.Errorf("après un palier : ETA=%.1f attendu ≈%.1f", r.ETA, want)
	}
	// a limit change is a regime change: the ETA follows at once
	r = s.step(1<<20, 1<<20)
	want = (float64(size) - s.done) / (1 << 20)
	if math.Abs(r.ETA-want) > 0.1*want {
		t.Errorf("après limite : ETA=%.1f attendu ≈%.1f", r.ETA, want)
	}
}

func TestETAPause(t *testing.T) {
	size := int64(200 << 20)
	s := newSim(size)
	var r etaResult
	for i := 0; i < 40; i++ {
		r = s.step(4<<20, 0)
	}
	before := r.ETA
	// stall: no data for 10 s — the ETA rises gradually and stays finite
	var last float64
	for i := 0; i < 40; i++ {
		r = s.step(0, 0)
		if math.IsInf(r.ETA, 0) || math.IsNaN(r.ETA) || r.ETA < 0 {
			t.Fatalf("ETA invalide pendant un blocage : %v", r.ETA)
		}
		if last > 0 && r.ETA > last*(1+0.15/4)+0.26 {
			t.Fatalf("ETA saute pendant un blocage : %.1f -> %.1f", last, r.ETA)
		}
		last = r.ETA
	}
	if last <= before {
		t.Errorf("l'ETA devrait augmenter pendant un blocage (%.1f -> %.1f)", before, last)
	}
	// pause + resume = new session: "calculating", then a fresh estimate
	s.e = newETA(s.now.Add(time.Minute), int64(s.done))
	s.now = s.now.Add(time.Minute)
	if r = s.step(8<<20, 0); r.ETA != ETACalculating {
		t.Fatalf("reprise : ETA=%v attendu -2", r.ETA)
	}
	for i := 0; i < 20; i++ {
		r = s.step(8<<20, 0)
	}
	want := (float64(size) - s.done) / (8 << 20)
	if math.Abs(r.ETA-want) > 0.1*want+1 {
		t.Errorf("après reprise : ETA=%.1f attendu ≈%.1f", r.ETA, want)
	}
}
