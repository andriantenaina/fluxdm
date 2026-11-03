// Package core orchestrates all downloads (HTTP, FTP, HLS, yt-dlp, torrent),
// queues, categories, settings and events for the UI and the browser bridge.
package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

// MainQueue is the default queue; it always runs.
const MainQueue = "main"

// Progress is sent to the UI several times per second.
type Progress struct {
	ID         string           `json:"id"`
	Size       int64            `json:"size"`
	Downloaded int64            `json:"downloaded"`
	Speed      float64          `json:"speed"`
	ETA        float64          `json:"eta"` // seconds, -1 unknown
	Conns      int              `json:"conns"`
	Segments   []engine.Segment `json:"segments,omitempty"`
	Extra      map[string]any   `json:"extra,omitempty"` // kind-specific (peers, fragments...)
}

// Event is broadcast to subscribers (UI, browser bridge).
type Event struct {
	Type     string      `json:"type"` // added | updated | removed | progress | completed | failed | settings
	Item     *store.Item `json:"item,omitempty"`
	ID       string      `json:"id,omitempty"`
	Progress *Progress   `json:"progress,omitempty"`
}

// Reporter lets a runner publish information about its item.
type Reporter interface {
	// Info updates file name / final path / size once known.
	Info(fileName, path string, size int64)
	// Title sets a human title (videos).
	Title(title string)
	// Progress publishes live progress (ID is filled in automatically).
	Progress(p Progress)
	// SaveState persists resume state (JSON-serializable) and downloaded bytes.
	SaveState(state any, downloaded int64)
	// Status sets an intermediate status (e.g. processing during a merge).
	Status(status string)
}

// Runner downloads one item until done, failure, or ctx cancellation (pause).
type Runner func(ctx context.Context, m *Manager, it *store.Item, rep Reporter) error

// AddRequest describes a new download.
type AddRequest struct {
	URL         string            `json:"url"`
	PageURL     string            `json:"pageUrl,omitempty"`
	Title       string            `json:"title,omitempty"`
	FileName    string            `json:"fileName,omitempty"`
	Dir         string            `json:"dir,omitempty"`
	Category    string            `json:"category,omitempty"`
	Queue       string            `json:"queue,omitempty"`
	Kind        string            `json:"kind,omitempty"` // empty = auto-detect
	Headers     map[string]string `json:"headers,omitempty"`
	Options     map[string]any    `json:"options,omitempty"`
	Start       bool              `json:"start"` // false = "download later"
	Size        int64             `json:"size,omitempty"`
	Description string            `json:"description,omitempty"`
}

// QueueState is the runtime state of a queue.
type QueueState struct {
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	MaxActive int    `json:"maxActive"`
}

type running struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager is the central download controller.
type Manager struct {
	DataDir string
	Store   *store.Store

	mu       sync.Mutex
	settings Settings
	items    map[string]*store.Item
	running  map[string]*running
	queues   map[string]*QueueState
	runners  map[string]Runner
	subs     map[int]func(Event)
	subSeq   int
	live     map[string]*engine.Download // running HTTP downloads (live tuning)

	client  *http.Client
	limiter *engine.Limiter

	// Classify may override kind auto-detection (e.g. send YouTube pages to yt-dlp).
	Classify func(rawURL string) string
	// OnCompleted is called after an item completes (antivirus, notifications...).
	OnCompleted []func(it *store.Item)
}

// NewManager opens the database in dataDir and loads settings and items.
func NewManager(dataDir string) (*Manager, error) {
	st, err := store.Open(filepath.Join(dataDir, "fluxdm.db"))
	if err != nil {
		return nil, err
	}
	m := &Manager{
		DataDir: dataDir,
		Store:   st,
		items:   map[string]*store.Item{},
		running: map[string]*running{},
		queues:  map[string]*QueueState{MainQueue: {Name: MainQueue, Running: true}},
		runners: map[string]Runner{},
		subs:    map[int]func(Event){},
		live:    map[string]*engine.Download{},
		limiter: engine.NewLimiter(0),
	}
	s := DefaultSettings()
	if _, err := st.GetSetting("settings", &s); err != nil {
		return nil, err
	}
	s.normalize()
	m.settings = s
	var qs []*QueueState
	if ok, _ := st.GetSetting("queues", &qs); ok {
		for _, q := range qs {
			if q.Name != MainQueue {
				m.queues[q.Name] = q
			}
		}
	}
	m.applySettingsLocked()
	items, err := st.ListItems()
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Status == store.StatusDownloading || it.Status == store.StatusProcessing {
			it.Status = store.StatusQueued // interrupted by a crash/shutdown: resume
		}
		m.items[it.ID] = it
	}
	m.runners[store.KindHTTP] = httpRunner
	return m, nil
}

// Close pauses everything and closes the database.
func (m *Manager) Close() {
	m.PauseAll()
	m.Store.Close()
}

// Register installs a runner for a kind (hls, ytdlp, torrent, ftp...).
func (m *Manager) Register(kind string, r Runner) {
	m.mu.Lock()
	m.runners[kind] = r
	m.mu.Unlock()
}

// Runner returns the runner registered for kind (nil if none).
func (m *Manager) Runner(kind string) Runner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runners[kind]
}

// Subscribe registers an event listener; call the returned func to unsubscribe.
func (m *Manager) Subscribe(fn func(Event)) func() {
	m.mu.Lock()
	m.subSeq++
	id := m.subSeq
	m.subs[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

func (m *Manager) emit(ev Event) {
	m.mu.Lock()
	fns := make([]func(Event), 0, len(m.subs))
	for _, f := range m.subs {
		fns = append(fns, f)
	}
	m.mu.Unlock()
	for _, f := range fns {
		f(ev)
	}
}

// HTTPClient returns the shared HTTP client (proxy/TLS settings applied).
func (m *Manager) HTTPClient() *http.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client
}

// Limiter returns the global speed limiter.
func (m *Manager) Limiter() *engine.Limiter { return m.limiter }

// EngineConfig returns the engine configuration derived from settings.
func (m *Manager) EngineConfig() engine.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engineConfigLocked()
}

func (m *Manager) engineConfigLocked() engine.Config {
	c := engine.DefaultConfig()
	s := m.settings
	c.MaxConns = s.MaxConns
	c.Proxy = s.Proxy
	c.AllowHTTP2 = s.AllowHTTP2
	c.InsecureTLS = s.InsecureTLS
	if s.UserAgent != "" {
		c.UserAgent = s.UserAgent
	}
	c.SpeedLimiter = m.limiter
	return c
}

func (m *Manager) applySettingsLocked() {
	m.limiter.SetRate(m.settings.SpeedLimit)
	cl, err := engine.NewHTTPClient(m.engineConfigLocked())
	if err != nil {
		cfg := m.engineConfigLocked()
		cfg.Proxy = ""
		cl, _ = engine.NewHTTPClient(cfg)
	}
	m.client = cl
}

// Settings returns a copy of the settings.
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// SetSettings saves and applies new settings.
func (m *Manager) SetSettings(s Settings) error {
	s.normalize()
	if s.Proxy != "" {
		if _, err := url.Parse(s.Proxy); err != nil {
			return fmt.Errorf("proxy invalide : %w", err)
		}
	}
	if err := m.Store.SetSetting("settings", s); err != nil {
		return err
	}
	m.mu.Lock()
	m.settings = s
	m.applySettingsLocked()
	m.mu.Unlock()
	m.emit(Event{Type: "settings"})
	m.pump()
	return nil
}

// SetSpeedLimit changes the global speed limit immediately (bytes/s, 0 = unlimited).
func (m *Manager) SetSpeedLimit(bps int64) error {
	s := m.Settings()
	s.SpeedLimit = bps
	return m.SetSettings(s)
}

// List returns all items sorted by creation date.
func (m *Manager) List() []*store.Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*store.Item, 0, len(m.items))
	for _, it := range m.items {
		c := *it
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Get returns a copy of an item.
func (m *Manager) Get(id string) (*store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	c := *it
	return &c, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DetectKind guesses how a URL must be downloaded.
func (m *Manager) DetectKind(rawURL string) string {
	if m.Classify != nil {
		if k := m.Classify(rawURL); k != "" {
			return k
		}
	}
	return DetectKind(rawURL)
}

// DetectKind is the default URL classifier.
func DetectKind(rawURL string) string {
	low := strings.ToLower(rawURL)
	if strings.HasPrefix(low, "magnet:") {
		return store.KindTorrent
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return store.KindHTTP
	}
	if u.Scheme == "ftp" || u.Scheme == "ftps" {
		return store.KindFTP
	}
	p := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(p, ".m3u8"):
		return store.KindHLS
	case strings.HasSuffix(p, ".mpd"):
		return store.KindDASH
	case strings.HasSuffix(p, ".torrent"):
		return store.KindTorrent
	}
	return store.KindHTTP
}

// Add creates a new download.
func (m *Manager) Add(req AddRequest) (*store.Item, error) {
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		return nil, errors.New("URL vide")
	}
	if fi, err := os.Stat(req.URL); err == nil && !fi.IsDir() && strings.HasSuffix(strings.ToLower(req.URL), ".torrent") {
		req.Kind = store.KindTorrent // local .torrent file
	} else if !strings.HasPrefix(strings.ToLower(req.URL), "magnet:") {
		u, err := url.Parse(req.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("URL invalide : %s", req.URL)
		}
	}
	kind := req.Kind
	if kind == "" {
		kind = m.DetectKind(req.URL)
	}
	m.mu.Lock()
	if _, ok := m.runners[kind]; !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("type de téléchargement non pris en charge : %s", kind)
	}
	s := m.settings
	m.mu.Unlock()

	name := req.FileName
	if name == "" && kind == store.KindHTTP {
		name = engine.FileNameFromResponse(req.URL, "", "")
		if name == "download" {
			name = ""
		}
	}
	if name != "" {
		name = engine.SanitizeFileName(name)
	}
	cat := req.Category
	if cat == "" {
		switch kind {
		case store.KindHLS, store.KindDASH, store.KindYtdlp:
			cat = "Vidéos"
			if v, _ := req.Options["audioOnly"].(bool); v {
				cat = "Musique"
			}
		case store.KindTorrent:
			cat = "Torrents"
		default:
			cat = s.CategoryFor(name)
		}
	}
	dir := req.Dir
	if dir == "" {
		dir = s.DirFor(cat)
	}
	queue := req.Queue
	if queue == "" {
		queue = MainQueue
	}
	it := &store.Item{
		ID:          newID(),
		Kind:        kind,
		URL:         req.URL,
		PageURL:     req.PageURL,
		Title:       req.Title,
		FileName:    name,
		Dir:         dir,
		Category:    cat,
		Queue:       queue,
		Status:      store.StatusPaused,
		Size:        req.Size,
		Description: req.Description,
		Headers:     req.Headers,
		Options:     req.Options,
		CreatedAt:   time.Now(),
	}
	if req.Start {
		it.Status = store.StatusQueued
	}
	if err := m.Store.SaveItem(it); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.items[it.ID] = it
	c := *it
	m.mu.Unlock()
	m.emit(Event{Type: "added", Item: &c})
	if req.Start {
		if queue == MainQueue {
			m.pump()
		} else {
			// explicit "start now" from the add dialog
			_ = m.Start(it.ID)
		}
	}
	return &c, nil
}

func (m *Manager) update(it *store.Item, persist bool) {
	m.mu.Lock()
	c := *it
	m.mu.Unlock()
	if persist {
		_ = m.Store.SaveItem(&c)
	}
	m.emit(Event{Type: "updated", Item: &c})
}

// Start runs an item now (regardless of queue limits).
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	if _, isRunning := m.running[id]; isRunning || it.Status == store.StatusCompleted {
		m.mu.Unlock()
		return nil
	}
	runner := m.runners[it.Kind]
	if runner == nil {
		m.mu.Unlock()
		return fmt.Errorf("type non pris en charge : %s", it.Kind)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{})}
	m.running[id] = r
	it.Status = store.StatusDownloading
	it.Error = ""
	m.mu.Unlock()
	m.update(it, true)

	go m.run(ctx, it, runner, r)
	return nil
}

func (m *Manager) run(ctx context.Context, it *store.Item, runner Runner, r *running) {
	rep := &reporter{m: m, it: it}
	err := runner(ctx, m, it, rep)

	m.mu.Lock()
	delete(m.running, it.ID)
	_, stillExists := m.items[it.ID]
	switch {
	case !stillExists:
	case err == nil:
		it.Status = store.StatusCompleted
		now := time.Now()
		it.CompletedAt = &now
		if it.Size > 0 {
			it.Downloaded = it.Size
		}
	case errors.Is(err, context.Canceled):
		if it.Status != store.StatusQueued {
			it.Status = store.StatusPaused
		}
	default:
		it.Status = store.StatusError
		it.Error = err.Error()
	}
	status := it.Status
	m.mu.Unlock()
	close(r.done)

	if !stillExists {
		return
	}
	m.update(it, true)
	switch status {
	case store.StatusCompleted:
		c := *it
		m.emit(Event{Type: "completed", Item: &c})
		for _, f := range m.OnCompleted {
			go f(&c)
		}
		m.runAntivirus(&c)
	case store.StatusError:
		c := *it
		m.emit(Event{Type: "failed", Item: &c})
	}
	m.pump()
}

func (m *Manager) runAntivirus(it *store.Item) {
	s := m.Settings()
	if s.AntivirusPath == "" || it.Path == "" {
		return
	}
	args := strings.Fields(s.AntivirusArgs)
	found := false
	for i, a := range args {
		if strings.Contains(a, "{file}") {
			args[i] = strings.ReplaceAll(a, "{file}", it.Path)
			found = true
		}
	}
	if !found {
		args = append(args, it.Path)
	}
	go func() {
		cmd := exec.Command(s.AntivirusPath, args...)
		hideWindow(cmd)
		_ = cmd.Run()
	}()
}

// Pause stops an item (it can be resumed later).
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	r := m.running[id]
	if r == nil {
		if it.Status == store.StatusQueued {
			it.Status = store.StatusPaused
			m.mu.Unlock()
			m.update(it, true)
			return nil
		}
		m.mu.Unlock()
		return nil
	}
	it.Status = store.StatusPaused
	m.mu.Unlock()
	r.cancel()
	<-r.done
	return nil
}

// Enqueue marks an item to be started by its queue.
func (m *Manager) Enqueue(id string) error {
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	if it.Status == store.StatusPaused || it.Status == store.StatusError {
		it.Status = store.StatusQueued
	}
	m.mu.Unlock()
	m.update(it, true)
	m.pump()
	return nil
}

// PauseAll pauses every running item.
func (m *Manager) PauseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.running))
	for id := range m.running {
		ids = append(ids, id)
	}
	for _, it := range m.items {
		if it.Status == store.StatusQueued {
			it.Status = store.StatusPaused
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Pause(id)
	}
}

// StartAll queues every paused/failed item.
func (m *Manager) StartAll() {
	m.mu.Lock()
	for _, it := range m.items {
		if it.Status == store.StatusPaused || it.Status == store.StatusError {
			it.Status = store.StatusQueued
			_ = m.Store.SaveItem(it)
		}
	}
	m.mu.Unlock()
	m.pump()
}

// Remove deletes an item; deleteFile also removes the downloaded/partial file.
func (m *Manager) Remove(id string, deleteFile bool) error {
	_ = m.Pause(id)
	m.mu.Lock()
	it, ok := m.items[id]
	if ok {
		delete(m.items, id)
	}
	m.mu.Unlock()
	if !ok {
		return store.ErrNotFound
	}
	if it.Path != "" {
		_ = os.Remove(it.Path + engine.PartExt)
		if deleteFile {
			_ = os.Remove(it.Path)
		}
	}
	cleanupTemp(m.DataDir, it.ID)
	if err := m.Store.DeleteItem(id); err != nil {
		return err
	}
	m.emit(Event{Type: "removed", ID: id})
	return nil
}

// Redownload restarts an item from zero.
func (m *Manager) Redownload(id string) error {
	_ = m.Pause(id)
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	if it.Path != "" {
		_ = os.Remove(it.Path + engine.PartExt)
	}
	cleanupTemp(m.DataDir, it.ID)
	it.State = nil
	it.Path = ""
	it.Downloaded = 0
	it.Status = store.StatusPaused
	it.CompletedAt = nil
	m.mu.Unlock()
	m.update(it, true)
	return m.Start(id)
}

// UpdateURL replaces the address of an item (refresh an expired link) keeping progress.
func (m *Manager) UpdateURL(id, newURL string, headers map[string]string) error {
	wasRunning := false
	m.mu.Lock()
	_, wasRunning = m.running[id]
	m.mu.Unlock()
	_ = m.Pause(id)
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	it.URL = newURL
	if headers != nil {
		it.Headers = headers
	}
	if it.Kind == store.KindHTTP && len(it.State) > 0 {
		var snap engine.Snapshot
		if json.Unmarshal(it.State, &snap) == nil {
			snap.URL, snap.FinalURL = newURL, ""
			if headers != nil {
				snap.Headers = headers
			}
			it.State, _ = json.Marshal(snap)
		}
	}
	m.mu.Unlock()
	m.update(it, true)
	if wasRunning || it.Status == store.StatusError {
		return m.Start(id)
	}
	return nil
}

// SetItemOptions changes per-item options (e.g. "connections", "speedLimit").
func (m *Manager) SetItemOptions(id string, opts map[string]any) error {
	m.mu.Lock()
	it, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	if it.Options == nil {
		it.Options = map[string]any{}
	}
	for k, v := range opts {
		it.Options[k] = v
	}
	d := m.live[id]
	m.mu.Unlock()
	if d != nil { // apply immediately to a running HTTP download
		if _, ok := opts["speedLimit"]; ok && d.Limiter != nil {
			d.Limiter.SetRate(int64(optInt(opts, "speedLimit")))
		}
		if n := optInt(opts, "connections"); n > 0 {
			d.SetMaxConns(n)
		}
	}
	m.update(it, true)
	return nil
}

// Queues returns all queues.
func (m *Manager) Queues() []QueueState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]QueueState, 0, len(m.queues))
	for _, q := range m.queues {
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == MainQueue {
			return true
		}
		if out[j].Name == MainQueue {
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// SaveQueue creates or updates a queue.
func (m *Manager) SaveQueue(q QueueState) error {
	if q.Name == "" {
		return errors.New("nom de file vide")
	}
	m.mu.Lock()
	if q.Name == MainQueue {
		m.queues[MainQueue].MaxActive = q.MaxActive
	} else {
		m.queues[q.Name] = &q
	}
	err := m.saveQueuesLocked()
	m.mu.Unlock()
	m.pump()
	return err
}

// DeleteQueue removes a queue; its items move to the main queue.
func (m *Manager) DeleteQueue(name string) error {
	if name == MainQueue {
		return errors.New("la file principale ne peut pas être supprimée")
	}
	m.mu.Lock()
	delete(m.queues, name)
	for _, it := range m.items {
		if it.Queue == name {
			it.Queue = MainQueue
			_ = m.Store.SaveItem(it)
		}
	}
	err := m.saveQueuesLocked()
	m.mu.Unlock()
	return err
}

func (m *Manager) saveQueuesLocked() error {
	var qs []*QueueState
	for _, q := range m.queues {
		qs = append(qs, q)
	}
	return m.Store.SetSetting("queues", qs)
}

// MoveToQueue changes the queue of items.
func (m *Manager) MoveToQueue(ids []string, queue string) {
	m.mu.Lock()
	if _, ok := m.queues[queue]; !ok {
		queue = MainQueue
	}
	var changed []*store.Item
	for _, id := range ids {
		if it, ok := m.items[id]; ok {
			it.Queue = queue
			changed = append(changed, it)
		}
	}
	m.mu.Unlock()
	for _, it := range changed {
		m.update(it, true)
	}
}

// StartQueue starts a queue: its queued/paused items run up to its limit.
func (m *Manager) StartQueue(name string) {
	m.mu.Lock()
	q, ok := m.queues[name]
	if ok {
		q.Running = true
		for _, it := range m.items {
			if it.Queue == name && (it.Status == store.StatusPaused || it.Status == store.StatusError) {
				it.Status = store.StatusQueued
				_ = m.Store.SaveItem(it)
			}
		}
	}
	m.mu.Unlock()
	m.pump()
}

// StopQueue stops a queue and pauses its running items.
func (m *Manager) StopQueue(name string) {
	m.mu.Lock()
	if q, ok := m.queues[name]; ok && name != MainQueue {
		q.Running = false
	}
	var ids []string
	for id := range m.running {
		if it := m.items[id]; it != nil && it.Queue == name {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Pause(id)
	}
}

// QueueIdle reports whether a queue has nothing left to download.
func (m *Manager) QueueIdle(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.Queue == name && (it.Status == store.StatusQueued || it.Status == store.StatusDownloading || it.Status == store.StatusProcessing) {
			return false
		}
	}
	return true
}

// ActiveCount returns the number of running downloads.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}

// pump starts queued items while queue limits allow.
func (m *Manager) pump() {
	m.mu.Lock()
	activePerQueue := map[string]int{}
	for id := range m.running {
		if it := m.items[id]; it != nil {
			activePerQueue[it.Queue]++
		}
	}
	var toStart []string
	ordered := make([]*store.Item, 0, len(m.items))
	for _, it := range m.items {
		if it.Status == store.StatusQueued {
			ordered = append(ordered, it)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CreatedAt.Before(ordered[j].CreatedAt) })
	for _, it := range ordered {
		if _, r := m.running[it.ID]; r {
			continue
		}
		q := m.queues[it.Queue]
		if q == nil {
			q = m.queues[MainQueue]
		}
		if !q.Running {
			continue
		}
		limit := q.MaxActive
		if limit <= 0 {
			limit = m.settings.MaxActive
		}
		if activePerQueue[q.Name] >= limit {
			continue
		}
		activePerQueue[q.Name]++
		toStart = append(toStart, it.ID)
	}
	m.mu.Unlock()
	for _, id := range toStart {
		_ = m.Start(id)
	}
}

// Resume restarts the queues after application launch.
func (m *Manager) Resume() { m.pump() }

// TempDir returns a per-item working directory (HLS fragments, yt-dlp parts...).
func (m *Manager) TempDir(id string) string {
	return filepath.Join(m.DataDir, "temp", id)
}

func cleanupTemp(dataDir, id string) {
	_ = os.RemoveAll(filepath.Join(dataDir, "temp", id))
}

// reporter implements Reporter for one item.
type reporter struct {
	m        *Manager
	it       *store.Item
	lastSave time.Time
}

func (r *reporter) Info(fileName, path string, size int64) {
	r.m.mu.Lock()
	if fileName != "" {
		r.it.FileName = fileName
	}
	if path != "" {
		r.it.Path = path
		r.it.Dir = filepath.Dir(path)
	}
	if size != 0 {
		r.it.Size = size
	}
	if r.it.Category == "" {
		r.it.Category = r.m.settings.CategoryFor(r.it.FileName)
	}
	r.m.mu.Unlock()
	r.m.update(r.it, true)
}

func (r *reporter) Title(title string) {
	r.m.mu.Lock()
	r.it.Title = title
	r.m.mu.Unlock()
	r.m.update(r.it, true)
}

func (r *reporter) Progress(p Progress) {
	p.ID = r.it.ID
	if p.ETA == 0 {
		p.ETA = -1
		if p.Speed > 1 && p.Size > 0 {
			p.ETA = float64(p.Size-p.Downloaded) / p.Speed
		}
	}
	r.m.mu.Lock()
	r.it.Downloaded = p.Downloaded
	if p.Size > 0 {
		r.it.Size = p.Size
	}
	r.m.mu.Unlock()
	r.m.emit(Event{Type: "progress", ID: r.it.ID, Progress: &p})
}

func (r *reporter) SaveState(state any, downloaded int64) {
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	r.m.mu.Lock()
	r.it.State = raw
	if downloaded > 0 {
		r.it.Downloaded = downloaded
	}
	c := *r.it
	r.m.mu.Unlock()
	_ = r.m.Store.SaveItem(&c)
}

func (r *reporter) Status(status string) {
	r.m.mu.Lock()
	r.it.Status = status
	r.m.mu.Unlock()
	r.m.update(r.it, true)
}
