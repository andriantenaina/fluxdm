// Package scheduler starts/stops queues at given times, runs an action when a
// scheduled queue finishes (quit, sleep, hibernate, shutdown), enforces a
// download quota and periodically re-synchronizes files.
package scheduler

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

// Schedule controls one queue.
type Schedule struct {
	Queue     string `json:"queue"`
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"startTime"` // "HH:MM" or ""
	StopTime  string `json:"stopTime"`  // "HH:MM" or ""
	Days      []int  `json:"days"`      // 0 = Sunday ... 6; empty = every day
	Once      bool   `json:"once"`      // disable after the next start
	OnDone    string `json:"onDone"`    // none | quit | sleep | hibernate | shutdown
}

// Quota limits the amount downloaded in a time window (fair-access connections).
type Quota struct {
	Enabled bool  `json:"enabled"`
	MB      int64 `json:"mb"`
	Hours   int   `json:"hours"`
}

// Config is persisted under the "scheduler" setting.
type Config struct {
	Schedules        []Schedule `json:"schedules"`
	Quota            Quota      `json:"quota"`
	SyncEveryMinutes int        `json:"syncEveryMinutes"` // periodic file synchronization
}

// Status is shown in the UI.
type Status struct {
	Config      Config  `json:"config"`
	QuotaUsedMB float64 `json:"quotaUsedMb"`
	QuotaPaused bool    `json:"quotaPaused"`
	PendingDone string  `json:"pendingDone,omitempty"`
}

// Scheduler runs in the background.
type Scheduler struct {
	m *core.Manager
	// Quit is called for the "quit" action.
	Quit func()
	// Notify shows a message to the user (optional).
	Notify func(title, msg string)

	mu          sync.Mutex
	cfg         Config
	fired       map[string]string // schedule key -> date+time already fired
	started     map[string]bool   // queues started by the scheduler
	startedAt   map[string]time.Time
	buckets     map[int64]int64 // minute -> bytes
	lastBytes   map[string]int64
	quotaPaused bool
	lastSync    map[string]time.Time
}

// New loads the scheduler configuration.
func New(m *core.Manager) *Scheduler {
	s := &Scheduler{m: m, fired: map[string]string{}, started: map[string]bool{}, startedAt: map[string]time.Time{},
		buckets: map[int64]int64{}, lastBytes: map[string]int64{}, lastSync: map[string]time.Time{}}
	_, _ = m.Store.GetSetting("scheduler", &s.cfg)
	if s.cfg.Quota.Hours <= 0 {
		s.cfg.Quota.Hours = 1
	}
	return s
}

// Config returns the configuration.
func (s *Scheduler) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Save stores a new configuration.
func (s *Scheduler) Save(c Config) error {
	if c.Quota.Hours <= 0 {
		c.Quota.Hours = 1
	}
	if err := s.m.Store.SetSetting("scheduler", c); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = c
	s.mu.Unlock()
	return nil
}

// Status returns config and live counters.
func (s *Scheduler) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Config: s.cfg, QuotaUsedMB: float64(s.windowBytesLocked()) / (1 << 20), QuotaPaused: s.quotaPaused}
}

// Run is the background loop (stops when stop is closed).
func (s *Scheduler) Run(stop <-chan struct{}) {
	unsub := s.m.Subscribe(s.onEvent)
	defer unsub()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			s.tick(now)
		}
	}
}

func (s *Scheduler) onEvent(ev core.Event) {
	if ev.Type != "progress" || ev.Progress == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.lastBytes[ev.ID]
	s.lastBytes[ev.ID] = ev.Progress.Downloaded
	if !ok || ev.Progress.Downloaded < prev {
		return
	}
	s.buckets[time.Now().Unix()/60] += ev.Progress.Downloaded - prev
}

func (s *Scheduler) windowBytesLocked() int64 {
	from := time.Now().Add(-time.Duration(s.cfg.Quota.Hours)*time.Hour).Unix() / 60
	var n int64
	for k, v := range s.buckets {
		if k < from {
			delete(s.buckets, k)
			continue
		}
		n += v
	}
	return n
}

func hhmm(t time.Time) string { return t.Format("15:04") }

func dayAllowed(days []int, wd time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == int(wd) {
			return true
		}
	}
	return false
}

func (s *Scheduler) tick(now time.Time) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	cur := hhmm(now)
	today := now.Format("2006-01-02")
	changed := false

	for i, sc := range cfg.Schedules {
		if !sc.Enabled || !dayAllowed(sc.Days, now.Weekday()) {
			continue
		}
		key := sc.Queue
		if sc.StartTime != "" && sc.StartTime == cur && s.firedOnce(key+"/start", today+cur) {
			s.m.StartQueue(sc.Queue)
			s.mu.Lock()
			s.started[sc.Queue] = true
			s.startedAt[sc.Queue] = now
			s.mu.Unlock()
			if sc.Once {
				cfg.Schedules[i].Enabled = false
				changed = true
			}
		}
		if sc.StopTime != "" && sc.StopTime == cur && s.firedOnce(key+"/stop", today+cur) {
			s.m.StopQueue(sc.Queue)
			s.mu.Lock()
			delete(s.started, sc.Queue)
			s.mu.Unlock()
		}
	}
	if changed {
		_ = s.Save(cfg)
	}

	// queue finished -> action
	for _, sc := range cfg.Schedules {
		s.mu.Lock()
		st := s.started[sc.Queue]
		since := time.Since(s.startedAt[sc.Queue])
		s.mu.Unlock()
		if !st || since < 20*time.Second || !s.m.QueueIdle(sc.Queue) {
			continue
		}
		s.mu.Lock()
		delete(s.started, sc.Queue)
		s.mu.Unlock()
		s.doAction(sc.OnDone)
	}

	s.checkQuota(cfg.Quota)
	if cfg.SyncEveryMinutes > 0 {
		s.syncFiles(time.Duration(cfg.SyncEveryMinutes) * time.Minute)
	}
}

func (s *Scheduler) firedOnce(key, stamp string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired[key] == stamp {
		return false
	}
	s.fired[key] = stamp
	return true
}

func (s *Scheduler) checkQuota(q Quota) {
	s.mu.Lock()
	used := s.windowBytesLocked()
	paused := s.quotaPaused
	s.mu.Unlock()
	if !q.Enabled || q.MB <= 0 {
		if paused {
			s.mu.Lock()
			s.quotaPaused = false
			s.mu.Unlock()
			s.m.StartAll()
		}
		return
	}
	limit := q.MB << 20
	switch {
	case !paused && used >= limit:
		s.mu.Lock()
		s.quotaPaused = true
		s.mu.Unlock()
		s.m.PauseAll()
		if s.Notify != nil {
			s.Notify("Quota atteint", "Les téléchargements reprendront automatiquement.")
		}
	case paused && used < limit*9/10:
		s.mu.Lock()
		s.quotaPaused = false
		s.mu.Unlock()
		s.m.StartAll()
	}
}

// syncFiles re-downloads completed HTTP items marked "sync" when the remote file changed.
func (s *Scheduler) syncFiles(every time.Duration) {
	for _, it := range s.m.List() {
		if it.Kind != store.KindHTTP || it.Status != store.StatusCompleted || !core.OptBool(it.Options, "sync") {
			continue
		}
		s.mu.Lock()
		last := s.lastSync[it.ID]
		if time.Since(last) < every {
			s.mu.Unlock()
			continue
		}
		s.lastSync[it.ID] = time.Now()
		s.mu.Unlock()
		go s.syncOne(it)
	}
}

func (s *Scheduler) syncOne(it *store.Item) {
	var snap engine.Snapshot
	if len(it.State) == 0 || json.Unmarshal(it.State, &snap) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl := s.m.HTTPClient()
	req, err := newHead(ctx, it.URL, it.Headers)
	if err != nil {
		return
	}
	resp, err := cl.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	changed := false
	if et := resp.Header.Get("ETag"); et != "" && snap.ETag != "" && et != snap.ETag {
		changed = true
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" && snap.LastModified != "" && lm != snap.LastModified {
		changed = true
	}
	if resp.ContentLength > 0 && snap.Size > 0 && resp.ContentLength != snap.Size {
		changed = true
	}
	if changed {
		_ = s.m.Redownload(it.ID)
	}
}

func (s *Scheduler) doAction(a string) {
	switch strings.ToLower(a) {
	case "", "none":
		return
	case "quit":
		if s.Quit != nil {
			s.Quit()
		}
		return
	}
	if s.Notify != nil {
		s.Notify("FluxDM", "File terminée : "+a)
	}
	cmd := powerCommand(a)
	if cmd != nil {
		core.HideWindow(cmd)
		_ = cmd.Start()
	}
}

var _ = exec.Command
