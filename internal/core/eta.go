package core

import (
	"math"
	"time"
)

// ETA sentinels sent to the UI.
const (
	ETAUnknown     = -1 // size unknown
	ETACalculating = -2 // not enough data yet (first 3 s or first MiB of a session)
)

const (
	etaWindow       = 10 * time.Second
	etaWarmup       = 3 * time.Second
	etaWarmupBytes  = 1 << 20
	etaMaxStepPerS  = 0.15 // displayed ETA moves at most 15 % per second...
	etaRegimeFactor = 0.5  // ...unless the speed regime changed (±50 % for 5 s)
	etaRegimeHold   = 5 * time.Second
	etaMax          = 30 * 24 * 3600.0
)

type etaSample struct {
	t time.Time
	b int64
}

// etaEstimator turns cumulative byte counts into a stable, reliable time
// remaining for one download session.
//
//   - The current speed is the least-squares slope of the cumulative bytes
//     over a 10 s sliding window (robust to the bursts of segmented and
//     fragment-based downloads, unlike an EWMA of instant speeds).
//   - It is blended with the average since the start of the current regime,
//     weighted by progress: early the window is trusted, late the average.
//   - An active speed limit caps the speed used.
//   - The displayed value ticks down one second per second while the speed is
//     stable and cannot move by more than 15 %/s, except on a regime change
//     (resume, limit change, lasting speed change), where it jumps.
//   - The confidence comes from the coefficient of variation of the speed
//     within the window.
type etaEstimator struct {
	start time.Time // session start
	base  int64     // bytes at session start

	regStart time.Time // start of the current speed regime (for the average)
	regBase  int64

	samples []etaSample

	shown    float64
	shownAt  time.Time
	hasShown bool

	limit        int64
	divergeSince time.Time
}

type etaResult struct {
	ETA        float64 // seconds, or ETAUnknown / ETACalculating
	Confidence float64 // 0..1
	AvgSpeed   float64 // bytes/s since the session start
	Speed      float64 // estimated current speed (bytes/s)
}

func newETA(now time.Time, downloaded int64) *etaEstimator {
	return &etaEstimator{start: now, base: downloaded, regStart: now, regBase: downloaded}
}

// slope returns the least-squares slope (bytes/s) of the window and the
// coefficient of variation of the speed between samples.
func (e *etaEstimator) slope() (rate, cv, span float64) {
	n := len(e.samples)
	if n < 2 {
		return 0, 1, 0
	}
	t0 := e.samples[0].t
	b0 := e.samples[0].b
	var sx, sy, sxx, sxy float64
	for _, s := range e.samples {
		x := s.t.Sub(t0).Seconds()
		y := float64(s.b - b0)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	fn := float64(n)
	den := fn*sxx - sx*sx
	span = e.samples[n-1].t.Sub(t0).Seconds()
	if den <= 0 || span <= 0 {
		return 0, 1, span
	}
	rate = (fn*sxy - sx*sy) / den
	if rate < 0 {
		rate = 0
	}
	// variation of the speed between consecutive samples
	var sum, sum2 float64
	var m int
	for i := 1; i < n; i++ {
		dt := e.samples[i].t.Sub(e.samples[i-1].t).Seconds()
		if dt <= 0 {
			continue
		}
		v := float64(e.samples[i].b-e.samples[i-1].b) / dt
		sum += v
		sum2 += v * v
		m++
	}
	if m == 0 {
		return rate, 1, span
	}
	mean := sum / float64(m)
	if mean <= 0 {
		return rate, 1, span
	}
	variance := math.Max(sum2/float64(m)-mean*mean, 0)
	cv = math.Sqrt(variance) / mean
	return rate, cv, span
}

// update feeds a new measurement and returns the estimate.
// limit is the effective speed limit in bytes/s (0 = none).
func (e *etaEstimator) update(now time.Time, downloaded, size, limit int64) etaResult {
	if downloaded < e.base { // restarted from zero (new phase, redownload)
		*e = *newETA(now, downloaded)
	}
	e.samples = append(e.samples, etaSample{now, downloaded})
	cut := 0
	for cut < len(e.samples)-2 && now.Sub(e.samples[cut].t) > etaWindow {
		cut++
	}
	if cut > 0 {
		e.samples = append(e.samples[:0], e.samples[cut:]...)
	}

	var res etaResult
	if el := now.Sub(e.start).Seconds(); el > 0 {
		res.AvgSpeed = float64(downloaded-e.base) / el
	}
	regime := false
	if limit != e.limit {
		e.limit = limit
		e.regStart, e.regBase = now, downloaded
		regime = true
	}

	win, cv, span := e.slope()
	if span < 1 {
		win = res.AvgSpeed
	}
	regAvg := win
	if el := now.Sub(e.regStart).Seconds(); el >= 2 {
		regAvg = float64(downloaded-e.regBase) / el
	}
	// lasting speed change: start a new regime from the window start
	if regAvg > 0 && span >= 2 && math.Abs(win-regAvg)/regAvg > etaRegimeFactor {
		if e.divergeSince.IsZero() {
			e.divergeSince = now
		} else if now.Sub(e.divergeSince) >= etaRegimeHold {
			e.regStart, e.regBase = e.samples[0].t, e.samples[0].b
			regAvg = win
			e.divergeSince = time.Time{}
			regime = true
		}
	} else {
		e.divergeSince = time.Time{}
	}

	conf := math.Max(0, math.Min(1, 1-cv))
	conf *= math.Min(1, span/(etaWindow.Seconds()/2))
	res.Confidence = conf

	if size <= 0 {
		res.ETA = ETAUnknown
		res.Speed = win
		return res
	}
	remaining := size - downloaded
	if remaining <= 0 {
		res.ETA, res.Confidence = 0, 1
		return res
	}
	if now.Sub(e.start) < etaWarmup || downloaded-e.base < etaWarmupBytes {
		res.ETA = ETACalculating
		res.Speed = win
		e.hasShown = false
		return res
	}

	p := math.Min(1, math.Max(0, float64(downloaded)/float64(size)))
	alpha := 0.6 * p // weight of the average: grows with progress
	rate := (1-alpha)*win + alpha*regAvg
	if limit > 0 && rate > float64(limit) {
		rate = float64(limit)
	}
	rate = math.Max(rate, math.Max(res.AvgSpeed*0.02, 1)) // stalled: still finite
	res.Speed = rate
	raw := math.Min(float64(remaining)/rate, etaMax)

	if !e.hasShown || regime || raw < 3 {
		e.shown = raw
	} else {
		dt := now.Sub(e.shownAt).Seconds()
		pred := math.Max(e.shown-dt, 0) // tick down 1 s/s
		step := math.Max(pred*etaMaxStepPerS*dt, dt)
		e.shown = pred + math.Max(-step, math.Min(step, raw-pred))
	}
	e.hasShown = true
	e.shownAt = now
	res.ETA = math.Max(e.shown, 0)
	return res
}
