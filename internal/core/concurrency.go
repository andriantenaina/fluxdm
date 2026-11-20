package core

import (
	"errors"
	"sort"
	"time"

	"fluxdm/internal/store"
)

// Tunable is a running download whose connections and limits can change live.
type Tunable interface {
	SetMaxConns(n int)
	SetSpeedLimit(bps int64)
	SetWeight(w float64)
	EffectiveLimit() int64
}

// SetLive registers a running download for live tuning (nil = remove).
// Runners in other packages (yt-dlp acceleration) use it too.
func (m *Manager) SetLive(id string, d Tunable) {
	if d == nil {
		m.live.Delete(id)
		return
	}
	m.live.Store(id, d)
}

func (m *Manager) liveDownload(id string) Tunable {
	if v, ok := m.live.Load(id); ok {
		return v.(Tunable)
	}
	return nil
}

// LatestProgress returns the last progress published by a running item (nil if none).
func (m *Manager) LatestProgress(id string) *Progress {
	if v, ok := m.prog.Load(id); ok {
		c := *v.(*Progress)
		return &c
	}
	return nil
}

// itemLimit returns the speed limit effectively applied to an item (bytes/s, 0 = none).
func (m *Manager) itemLimit(id string) int64 {
	if d := m.liveDownload(id); d != nil {
		return d.EffectiveLimit()
	}
	m.mu.Lock()
	var own int64
	if it := m.items[id]; it != nil {
		own = int64(optInt(it.Options, "speedLimit"))
	}
	m.mu.Unlock()
	lim := m.FairLimit()
	if own > 0 && (lim == 0 || own < lim) {
		lim = own
	}
	return lim
}

// autoCeilingLocked is the most simultaneous downloads AutoActive may reach.
func (m *Manager) autoCeilingLocked() int {
	c := m.settings.MaxActive
	for _, q := range m.queues {
		c = max(c, q.MaxActive)
	}
	if g := m.settings.MaxActiveGlobal; g > 0 && g < c {
		c = g
	}
	return max(c, 1)
}

// autoStartLocked is the AutoActive starting level.
func (m *Manager) autoStartLocked() int { return min(2, m.autoCeilingLocked()) }

func (m *Manager) autoCapLocked() int {
	return max(1, min(m.autoLimit, m.autoCeilingLocked()))
}

func (m *Manager) totalSpeed() float64 {
	var sp float64
	m.prog.Range(func(_, v any) bool {
		sp += v.(*Progress).Speed
		return true
	})
	return sp
}

// autoActiveLoop implements AutoActive: every 5 s, when all slots are busy
// and items wait, one more download is started if the total throughput rose
// by more than 10 % since the last addition, unless the next item's server
// already has no free connection.
func (m *Manager) autoActiveLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
		}
		m.mu.Lock()
		running := len(m.running)
		if running == 0 {
			m.autoLimit = m.autoStartLocked()
			m.autoLastSpeed = 0
			m.mu.Unlock()
			continue
		}
		add := false
		if m.settings.AutoActive {
			capN := m.autoCapLocked()
			var next *store.Item
			for _, it := range m.queuedLocked() {
				if q := m.queues[it.Queue]; q == nil || q.Running {
					next = it
					break
				}
			}
			if next != nil && running >= capN && capN < m.autoCeilingLocked() {
				speed := m.totalSpeed()
				if speed > m.autoLastSpeed*1.1 && !m.gate.Saturated(hostOf(next.URL)) {
					m.autoLimit = capN + 1
					m.autoLastSpeed = speed
					add = true
				}
			}
		}
		m.mu.Unlock()
		if add {
			m.pump()
		}
	}
}

// SetPriority sets the priority of items (-1 low, 0 normal, 1 high). A
// running download immediately gets the matching bandwidth weight.
func (m *Manager) SetPriority(ids []string, p int) {
	p = max(-1, min(1, p))
	m.mu.Lock()
	var changed []*store.Item
	for _, id := range ids {
		if it, ok := m.items[id]; ok && it.Priority != p {
			it.Priority = p
			changed = append(changed, it)
		}
	}
	m.mu.Unlock()
	for _, it := range changed {
		if d := m.liveDownload(it.ID); d != nil && m.Settings().FairShare {
			d.SetWeight(PriorityWeight(p))
		}
		m.update(it, true)
	}
	if len(changed) > 0 {
		m.pump()
	}
}

// Reorder moves item id just before beforeID in the same queue ("" = at the
// end). Moved items get an Order > 0 between their neighbours' keys (Order,
// or creation date in ms for items never moved); when there is no room left,
// the whole queue is renumbered.
func (m *Manager) Reorder(id, beforeID string) error {
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	var list []*store.Item
	for _, o := range m.items {
		if o.Queue == it.Queue && o.ID != id {
			list = append(list, o)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		ki, kj := list[i].SortKey(), list[j].SortKey()
		if ki != kj {
			return ki < kj
		}
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
	pos := len(list)
	if beforeID != "" {
		pos = -1
		for i, o := range list {
			if o.ID == beforeID {
				pos = i
				break
			}
		}
		if pos < 0 {
			m.mu.Unlock()
			return errors.New("élément de référence introuvable dans la même file")
		}
	}
	var key float64
	switch {
	case len(list) == 0:
		key = it.SortKey()
	case pos == len(list):
		key = list[pos-1].SortKey() + 1024
	case pos == 0:
		key = list[0].SortKey() - 1024
	default:
		key = (list[pos-1].SortKey() + list[pos].SortKey()) / 2
	}
	var changed []*store.Item
	valid := key > 0 &&
		(pos == 0 || key > list[pos-1].SortKey()) &&
		(pos == len(list) || key < list[pos].SortKey())
	if valid {
		it.Order = key
		changed = append(changed, it)
	} else { // no room between the neighbours: renumber the whole queue
		ordered := append(append(append([]*store.Item{}, list[:pos]...), it), list[pos:]...)
		for i, o := range ordered {
			if k := float64(i+1) * 1024; o.Order != k {
				o.Order = k
				changed = append(changed, o)
			}
		}
	}
	m.mu.Unlock()
	for _, c := range changed {
		m.update(c, true)
	}
	return nil
}

// Cancel stops and removes items, deleting their partial files (.fdmpart,
// temporary fragments). Completed files are never deleted.
func (m *Manager) Cancel(ids []string) error {
	var last error
	for _, id := range ids {
		if err := m.Remove(id, false); err != nil {
			last = err
		}
	}
	return last
}

// Totals summarizes all downloads for the status bar and the tray.
type Totals struct {
	Speed     float64 `json:"speed"`     // bytes/s
	Active    int     `json:"active"`    // running downloads
	Queued    int     `json:"queued"`    // waiting in a queue
	Remaining int64   `json:"remaining"` // bytes left over active+queued items of known size
	ETA       float64 `json:"eta"`       // seconds for all of them at the current total speed, -1 unknown
}

// Totals returns the global speed, counts and the queue ETA.
func (m *Manager) Totals() Totals {
	var t Totals
	var maxItemETA float64 // the queue cannot finish before its slowest running item
	m.mu.Lock()
	for id, it := range m.items {
		_, running := m.running[id]
		switch {
		case running:
			t.Active++
			size, done := it.Size, it.Downloaded
			if p := m.LatestProgress(id); p != nil {
				t.Speed += p.Speed
				if p.ETA > maxItemETA {
					maxItemETA = p.ETA
				}
				if p.Size > 0 {
					size = p.Size
				}
				done = p.Downloaded
			}
			if size > 0 && size > done {
				t.Remaining += size - done
			}
		case it.Status == store.StatusQueued:
			t.Queued++
			if it.Size > 0 && it.Size > it.Downloaded {
				t.Remaining += it.Size - it.Downloaded
			}
		}
	}
	m.mu.Unlock()
	t.ETA = -1
	if t.Remaining == 0 && t.Active+t.Queued > 0 {
		t.ETA = -1
	} else if t.Remaining == 0 {
		t.ETA = 0
	} else if t.Speed > 1 {
		t.ETA = float64(t.Remaining) / t.Speed
		if maxItemETA > t.ETA {
			t.ETA = maxItemETA
		}
	}
	return t
}
