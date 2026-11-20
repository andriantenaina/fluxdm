package engine

import (
	"context"
	"slices"
	"sort"
	"time"
)

// Segment is a byte range [Start, End) of the file; Pos is the next byte to write.
// End = -1 means "until EOF" (unknown size).
//
// A segment normally has one worker. Near the end of a download a second
// worker may duplicate the request of a straggling segment ("endgame"): both
// write the same bytes at the same offsets (harmless), Pos is the furthest
// contiguous position reached by either, and the first one to reach End
// cancels the other.
type Segment struct {
	Start int64 `json:"s"`
	End   int64 `json:"e"`
	Pos   int64 `json:"p"`

	holders   []*wctl   // workers currently downloading this segment
	rate      float64   // smoothed speed of its worker (bytes/s)
	since     time.Time // when the current worker took it
	lastWrite time.Time // last time data was written
}

// wctl controls one worker's current request (cancel = abort the request).
type wctl struct{ cancel context.CancelFunc }

func (s *Segment) remaining() int64 {
	if s.End < 0 {
		return 1 << 62
	}
	return s.End - s.Pos
}

func (s *Segment) complete() bool { return s.End >= 0 && s.Pos >= s.End }

// clean returns a copy without runtime fields (for snapshots and progress).
func (s *Segment) clean() Segment {
	return Segment{Start: s.Start, End: s.End, Pos: s.Pos}
}

func (s *Segment) attach(c *wctl, now time.Time, rate float64) {
	s.holders = append(s.holders, c)
	if len(s.holders) == 1 {
		s.since, s.lastWrite = now, now
		s.rate = rate
	}
}

func (s *Segment) detach(c *wctl) {
	for i, h := range s.holders {
		if h == c {
			s.holders = slices.Delete(s.holders, i, i+1)
			return
		}
	}
}

// effRate is the segment's speed, decayed when no data arrived recently
// (a stalled connection keeps its old rate otherwise).
func (s *Segment) effRate(now time.Time, typical float64) float64 {
	r := s.rate
	if r <= 0 {
		if now.Sub(s.since) < time.Second {
			return typical // just started: assume a normal connection
		}
		return 1
	}
	if age := now.Sub(s.lastWrite); age > time.Second {
		r *= float64(time.Second) / float64(age)
	}
	return r
}

// eta is the estimated time (s) the segment's worker needs to finish it.
func (s *Segment) eta(now time.Time, typical float64) float64 {
	r := s.effRate(now, typical)
	if r <= 0 {
		return 1e9
	}
	return float64(s.remaining()) / r
}

// typicalRateLocked returns the median of the latest speed of each connection
// of this download (a straggler does not drag it down).
func (d *Download) typicalRateLocked(extra float64) float64 {
	rs := make([]float64, 0, len(d.connRates)+1)
	for _, r := range d.connRates {
		if r > 0 {
			rs = append(rs, r)
		}
	}
	if extra > 0 {
		rs = append(rs, extra)
	}
	if len(rs) == 0 {
		return 0
	}
	sort.Float64s(rs)
	return rs[len(rs)/2]
}

// claimLocked returns a segment for a free connection (and registers c as its
// worker):
//  1. an unfinished segment nobody downloads;
//  2. otherwise the second half of the segment that will finish last
//     (estimated time remaining, not just size); near the end, halves down to
//     256 KiB are stolen;
//  3. otherwise (endgame) a duplicate request on a straggling segment that
//     needs more than twice the time a fresh connection would.
//
// myRate is the speed of the claiming worker (0 = unknown).
func (d *Download) claimLocked(c *wctl, myRate float64) *Segment {
	now := time.Now()
	for _, s := range d.segs {
		if len(s.holders) == 0 && s.remaining() > 0 {
			s.attach(c, now, myRate)
			return s
		}
	}
	if !d.snap.Resumable || d.snap.Size <= 0 {
		return nil
	}
	typical := d.typicalRateLocked(myRate)
	var best *Segment
	var bestETA float64
	for _, s := range d.segs {
		if s.End <= 0 || s.remaining() <= 0 {
			continue
		}
		if e := s.eta(now, typical); best == nil || e > bestETA {
			best, bestETA = s, e
		}
	}
	if best == nil {
		return nil
	}
	rem := best.remaining()
	small := min(d.cfg.MinSplit, endgameSplit)
	if rem >= 2*d.cfg.MinSplit || (rem >= 2*small && bestETA > 1) {
		mid := best.Pos + rem/2
		ns := &Segment{Start: mid, End: best.End, Pos: mid}
		best.End = mid
		ns.attach(c, now, myRate)
		d.insertAfterLocked(best, ns)
		return ns
	}
	if len(best.holders) == 1 && now.Sub(best.since) > time.Second && typical > 0 {
		fresh := float64(rem)/typical + 0.5 // a new request needs ~0.5 s to get going
		if bestETA > 2*fresh && bestETA > 1.5 {
			best.holders = append(best.holders, c)
			d.dups++
			return best
		}
	}
	return nil
}

// insertAfterLocked inserts ns right after s, keeping d.segs sorted by Start.
func (d *Download) insertAfterLocked(s, ns *Segment) {
	i := sort.Search(len(d.segs), func(i int) bool { return d.segs[i].Start > s.Start })
	// i is the first segment after s
	for i < len(d.segs) && d.segs[i].Start < ns.Start {
		i++
	}
	d.segs = slices.Insert(d.segs, i, ns)
}

// mergeLocked merges completed contiguous segments nobody works on, so the
// list (copied 4 times per second and saved every 2 s) stays short.
func (d *Download) mergeLocked() {
	if len(d.segs) < 2 {
		return
	}
	out := d.segs[:0]
	for _, s := range d.segs {
		if n := len(out); n > 0 {
			p := out[n-1]
			if p.complete() && s.complete() && len(p.holders) == 0 && len(s.holders) == 0 && p.End == s.Start {
				p.End = s.End
				p.Pos = s.End
				continue
			}
		}
		out = append(out, s)
	}
	clear(d.segs[len(out):])
	d.segs = out
}
