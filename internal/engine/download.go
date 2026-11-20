package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PartExt is appended to files while they are being downloaded.
const PartExt = ".fdmpart"

// Errors returned by Run.
var (
	ErrRemoteChanged = errors.New("le fichier distant a changé depuis le début du téléchargement")
	ErrNoRange       = errors.New("le serveur ne prend pas en charge la reprise")
)

// HTTPError is returned for non-success HTTP status codes.
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string { return "HTTP " + e.Status }

// Fatal reports whether retrying is pointless (expired link, forbidden, not found...).
func (e *HTTPError) Fatal() bool {
	c := e.StatusCode
	return c >= 400 && c < 500 && c != 408 && c != 425 && c != 429
}

// Snapshot is the persistable state of a download (used for resume).
type Snapshot struct {
	URL          string            `json:"url"`
	FinalURL     string            `json:"finalUrl,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Dir          string            `json:"dir"`
	FileName     string            `json:"fileName,omitempty"`
	Path         string            `json:"path,omitempty"` // final path, decided after probing
	Size         int64             `json:"size"`           // -1 = unknown, 0 = not probed yet
	Resumable    bool              `json:"resumable"`
	ETag         string            `json:"etag,omitempty"`
	LastModified string            `json:"lastModified,omitempty"`
	ContentType  string            `json:"contentType,omitempty"`
	Segments     []Segment         `json:"segments,omitempty"`
	Completed    bool              `json:"completed"`
	Mirrors      []string          `json:"mirrors,omitempty"` // other URLs of the same file
}

// Progress is reported periodically while downloading.
type Progress struct {
	Size       int64     `json:"size"`
	Downloaded int64     `json:"downloaded"`
	Speed      float64   `json:"speed"` // bytes/second (smoothed)
	Conns      int       `json:"conns"`
	Segments   []Segment `json:"segments"`
}

// Download is one HTTP(S) download. Create it with NewDownload, then call Run.
// Cancelling the context passed to Run pauses it; the Snapshot allows resuming.
type Download struct {
	cfg    Config
	client *http.Client

	// Limiter limits this download only (optional, in addition to cfg.SpeedLimiter).
	Limiter *Limiter
	// Callbacks (all optional).
	OnInfo     func(Snapshot) // after probing: file name, size, resumable known
	OnProgress func(Progress) // ~4 times per second
	OnSave     func(Snapshot) // every ~2 s and on exit, for persistence (data is on disk)
	OnConns    func(int)      // auto-tuning settled on a connection count

	urlMu   sync.Mutex
	urls    []string // primary + validated mirrors
	urlSeq  atomic.Uint32
	badURLs map[string]time.Time

	gateHost string

	mu        sync.Mutex
	snap      Snapshot
	segs      []*Segment // sorted by Start
	file      *os.File
	share     *Share
	active    int32
	maxConns  int32
	dups      int                  // endgame duplicate requests started
	connRates map[*float64]float64 // latest speed of each connection (live or ended)
	lastErr   error
	fatalErr  error
	cancel    context.CancelFunc
	done      chan struct{}

	speed     float64
	lastBytes int64
	lastTick  time.Time
}

// NewDownload creates a download from a (new or saved) snapshot.
func NewDownload(client *http.Client, cfg Config, snap Snapshot) *Download {
	cfg.normalize()
	d := &Download{cfg: cfg, client: client, snap: snap, maxConns: int32(cfg.MaxConns)}
	for i := range snap.Segments {
		s := snap.Segments[i].clean()
		d.segs = append(d.segs, &s)
	}
	return d
}

// SetMaxConns changes the number of connections while running.
func (d *Download) SetMaxConns(n int) {
	atomic.StoreInt32(&d.maxConns, int32(min(max(n, 1), 32)))
}

// Conns returns the number of connections currently open.
func (d *Download) Conns() int { return int(atomic.LoadInt32(&d.active)) }

// SetSpeedLimit changes this download's own limit (bytes/s, 0 = none).
// Limiter must have been set before Run.
func (d *Download) SetSpeedLimit(bps int64) {
	if d.Limiter != nil {
		d.Limiter.SetRate(bps)
	}
}

// SetWeight changes the fair-share weight (priority) while running.
func (d *Download) SetWeight(w float64) {
	d.mu.Lock()
	sh := d.share
	d.cfg.FairWeight = w
	d.mu.Unlock()
	sh.SetWeight(w)
}

// EffectiveLimit returns the speed limit currently applied to this download
// (bytes/s, 0 = none): its fair part of the global limit and its own limit.
func (d *Download) EffectiveLimit() int64 {
	d.mu.Lock()
	sh := d.share
	d.mu.Unlock()
	var lim int64
	if sh != nil {
		lim = sh.Alloc()
	} else {
		lim = d.cfg.SpeedLimiter.Rate()
	}
	if own := d.Limiter.Rate(); own > 0 && (lim == 0 || own < lim) {
		lim = own
	}
	return lim
}

// Snapshot returns a copy of the current persistable state.
func (d *Download) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snapshotLocked()
}

func (d *Download) snapshotLocked() Snapshot {
	s := d.snap
	s.Segments = make([]Segment, 0, len(d.segs))
	for _, sg := range d.segs {
		s.Segments = append(s.Segments, sg.clean())
	}
	return s
}

func hostKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// Run downloads until completion, error, or context cancellation (pause).
func (d *Download) Run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.cancel = cancel
	d.done = make(chan struct{}, 64)
	d.gateHost = hostKey(d.snap.URL)

	if d.snap.Completed {
		return nil
	}
	if d.cfg.FairWeight > 0 && d.cfg.SpeedLimiter != nil {
		sh := d.cfg.SpeedLimiter.NewShare(d.cfg.FairWeight)
		d.mu.Lock()
		d.share = sh
		d.mu.Unlock()
		defer func() {
			d.mu.Lock()
			d.share = nil
			d.mu.Unlock()
			sh.Close()
		}()
	}

	var firstBody io.ReadCloser
	firstRelease := noop
	if len(d.segs) == 0 || d.snap.Path == "" {
		rel, err := d.cfg.HostGate.Acquire(ctx, d.gateHost)
		if err != nil {
			return err
		}
		body, err := d.probe(ctx)
		if err != nil {
			rel()
			return err
		}
		firstBody, firstRelease = body, rel
		if d.OnInfo != nil {
			d.OnInfo(d.Snapshot())
		}
	} else if !d.snap.Resumable {
		// Cannot resume: restart from zero.
		d.segs = []*Segment{{Start: 0, End: d.snap.Size, Pos: 0}}
		if d.snap.Size <= 0 {
			d.segs[0].End = -1
		}
	}
	fail := func(err error) error {
		closeBody(firstBody)
		firstRelease()
		return err
	}

	if err := os.MkdirAll(filepath.Dir(d.snap.Path), 0o755); err != nil {
		return fail(err)
	}
	f, err := os.OpenFile(d.snap.Path+PartExt, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fail(err)
	}
	d.file = f
	defer f.Close()
	if d.snap.Size > 0 {
		if st, err := f.Stat(); err == nil && st.Size() != d.snap.Size {
			if st.Size() == 0 {
				_ = setSparse(f) // no synchronous zero-fill of the whole file
			}
			if err := f.Truncate(d.snap.Size); err != nil {
				return fail(err)
			}
		}
	}

	var wg sync.WaitGroup
	start := func(seg *Segment, c *wctl, wctx context.Context, body io.ReadCloser, release func()) {
		atomic.AddInt32(&d.active, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.workerLoop(ctx, seg, c, wctx, body, release)
			atomic.AddInt32(&d.active, -1)
			d.done <- struct{}{}
		}()
	}

	// Mirrors are validated in the background: the first connection starts
	// reading the probe response right away.
	d.setURLs([]string{d.targetURL()})
	go d.prepareMirrors(ctx)

	if firstBody != nil {
		wctx, wcancel := context.WithCancel(ctx)
		c := &wctl{cancel: wcancel}
		d.mu.Lock()
		firstSeg := d.segs[0]
		firstSeg.attach(c, time.Now(), 0)
		// Instant start: split the file right away into N equal parts so all
		// connections start at once (the first one reuses the probe response).
		if d.snap.Resumable && d.snap.Size > 0 {
			n := int64(atomic.LoadInt32(&d.maxConns))
			if maxN := d.snap.Size / d.cfg.MinSplit; n > maxN {
				n = max(maxN, 1)
			}
			part := d.snap.Size / n
			firstSeg.End = part
			for i := int64(1); i < n; i++ {
				end := (i + 1) * part
				if i == n-1 {
					end = d.snap.Size
				}
				d.segs = append(d.segs, &Segment{Start: i * part, End: end, Pos: i * part})
			}
		}
		d.mu.Unlock()
		start(firstSeg, c, wctx, firstBody, firstRelease)
	}

	d.lastTick = time.Now()
	d.lastBytes = d.downloaded()
	progressT := time.NewTicker(250 * time.Millisecond)
	defer progressT.Stop()
	spawnT := time.NewTicker(150 * time.Millisecond)
	defer spawnT.Stop()
	lastSave := time.Now()
	ticks := 0
	var ckWG sync.WaitGroup
	var saving atomic.Bool

	spawn := func() {
		for atomic.LoadInt32(&d.active) < atomic.LoadInt32(&d.maxConns) {
			release, ok := d.cfg.HostGate.TryAcquire(d.gateHost)
			if !ok {
				return // the host has no free connection: retry on the next tick
			}
			wctx, wcancel := context.WithCancel(ctx)
			c := &wctl{cancel: wcancel}
			d.mu.Lock()
			var seg *Segment
			if d.fatalErr == nil {
				seg = d.claimLocked(c, 0)
			}
			d.mu.Unlock()
			if seg == nil {
				wcancel()
				release()
				return
			}
			start(seg, c, wctx, nil, release)
		}
	}
	spawn()

	tn := tuner{floor: atomic.LoadInt32(&d.maxConns)}
	result := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-d.done:
				d.mu.Lock()
				fe := d.fatalErr
				d.mu.Unlock()
				if fe != nil {
					return fe
				}
				if d.complete() {
					return nil
				}
				spawn()
				if atomic.LoadInt32(&d.active) == 0 {
					d.mu.Lock()
					le := d.lastErr
					d.mu.Unlock()
					if le == nil {
						le = errors.New("téléchargement interrompu")
					}
					return le
				}
			case <-spawnT.C:
				spawn()
			case <-progressT.C:
				d.report()
				ticks++
				if ticks%8 == 0 { // every 2 s
					d.autoTune(&tn, spawn)
				}
				if time.Since(lastSave) > 2*time.Second && !saving.Load() {
					lastSave = time.Now()
					saving.Store(true)
					ckWG.Add(1)
					go func() { // fsync may take a while: keep the loop responsive
						defer ckWG.Done()
						defer saving.Store(false)
						d.checkpoint()
					}()
				}
			}
		}
	}()

	cancel()
	wg.Wait()
	ckWG.Wait()
	d.report()

	if result == nil {
		if err := f.Sync(); err != nil {
			return err
		}
		f.Close()
		final := d.snap.Path
		if exists(final) {
			final = UniquePath(filepath.Dir(final), filepath.Base(final))
		}
		if err := os.Rename(d.snap.Path+PartExt, final); err != nil {
			return err
		}
		d.mu.Lock()
		d.snap.Path = final
		d.snap.Completed = true
		d.mu.Unlock()
		d.save()
	} else {
		d.checkpoint()
	}
	if result != nil && parent.Err() != nil {
		return parent.Err()
	}
	return result
}

// tuner is the state of the connection auto-tuning.
type tuner struct {
	samples  []float64 // throughput every 2 s since the last change
	pending  bool      // a connection was added and is being evaluated
	base     float64   // average throughput before the addition
	addedAt  time.Time
	cooldown time.Time
	floor    int32 // never remove connections below the configured count
}

// autoTune tries one more connection when all are busy, keeps it if the
// average throughput rose by 5 % or more, and removes it otherwise.
func (d *Download) autoTune(t *tuner, spawn func()) {
	if !d.cfg.AutoConns || !d.snap.Resumable || d.snap.Size <= 0 {
		return
	}
	if lim := d.EffectiveLimit(); lim > 0 && d.speed >= 0.85*float64(lim) {
		t.samples = t.samples[:0] // limited by us, not by the network
		return
	}
	now := time.Now()
	t.samples = append(t.samples, d.speed)
	if len(t.samples) > 3 {
		t.samples = t.samples[1:]
	}
	var avg float64
	for _, s := range t.samples {
		avg += s
	}
	avg /= float64(len(t.samples))
	mc := atomic.LoadInt32(&d.maxConns)
	if t.pending {
		if now.Sub(t.addedAt) < 4*time.Second {
			return
		}
		t.pending = false
		t.samples = t.samples[:0]
		if avg < t.base*1.05 && mc > t.floor {
			if atomic.CompareAndSwapInt32(&d.maxConns, mc, mc-1) && d.OnConns != nil {
				d.OnConns(int(mc - 1))
			}
			t.cooldown = now.Add(20 * time.Second)
		} else if d.OnConns != nil {
			d.OnConns(int(mc))
		}
		return
	}
	if now.Before(t.cooldown) || len(t.samples) < 3 || mc >= 32 || atomic.LoadInt32(&d.active) < mc {
		return
	}
	if d.Size() > 0 && d.Size()-d.downloaded() < 8*d.cfg.MinSplit {
		return // almost done: not worth a new connection
	}
	t.base, t.pending, t.addedAt = avg, true, now
	t.samples = t.samples[:0]
	atomic.StoreInt32(&d.maxConns, mc+1)
	spawn()
}

// Size returns the file size (-1 unknown, 0 not probed).
func (d *Download) Size() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snap.Size
}

func closeBody(b io.ReadCloser) {
	if b != nil {
		b.Close()
	}
}

func (d *Download) save() {
	if d.OnSave != nil {
		d.OnSave(d.Snapshot())
	}
}

// checkpoint takes a snapshot, flushes the file, then publishes the snapshot:
// the saved positions never exceed what is really on disk.
func (d *Download) checkpoint() {
	snap := d.Snapshot()
	if d.file != nil {
		_ = d.file.Sync()
	}
	if d.OnSave != nil {
		d.OnSave(snap)
	}
}

func (d *Download) downloaded() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	var n int64
	for _, s := range d.segs {
		n += s.Pos - s.Start
	}
	return n
}

func (d *Download) complete() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.segs) == 0 {
		return false
	}
	for _, s := range d.segs {
		if !s.complete() {
			return false
		}
	}
	return true
}

func (d *Download) report() {
	now := time.Now()
	n := d.downloaded()
	dt := now.Sub(d.lastTick).Seconds()
	if dt > 0 {
		inst := float64(n-d.lastBytes) / dt
		if d.speed == 0 {
			d.speed = inst
		} else {
			d.speed = 0.7*d.speed + 0.3*inst
		}
	}
	d.lastTick, d.lastBytes = now, n
	if d.OnProgress == nil {
		return
	}
	d.mu.Lock()
	p := Progress{Size: d.snap.Size, Downloaded: n, Speed: d.speed, Conns: int(atomic.LoadInt32(&d.active))}
	p.Segments = make([]Segment, 0, len(d.segs))
	for _, s := range d.segs {
		p.Segments = append(p.Segments, s.clean())
	}
	d.mu.Unlock()
	d.OnProgress(p)
}

func (d *Download) setHeaders(req *http.Request) {
	for k, v := range d.snap.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", d.cfg.UserAgent)
	}
	req.Header.Set("Accept-Encoding", "identity")
}

func (d *Download) targetURL() string {
	if d.snap.FinalURL != "" {
		return d.snap.FinalURL
	}
	return d.snap.URL
}

func (d *Download) setURLs(u []string) {
	d.urlMu.Lock()
	d.urls = u
	d.urlMu.Unlock()
}

// prepareMirrors adds the mirrors that serve the same file (same size).
func (d *Download) prepareMirrors(ctx context.Context) {
	d.mu.Lock()
	mirrors, resumable, size, primary := d.snap.Mirrors, d.snap.Resumable, d.snap.Size, d.snap.URL
	headers := d.snap.Headers
	d.mu.Unlock()
	if len(mirrors) == 0 || !resumable || size <= 0 {
		return
	}
	var wg sync.WaitGroup
	for _, m := range mirrors {
		if m == "" || m == primary {
			continue
		}
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			info, err := Probe(pctx, d.client, d.cfg, m, headers)
			if err != nil || !info.Resumable || info.Size != size {
				return
			}
			d.urlMu.Lock()
			d.urls = append(d.urls, info.FinalURL)
			d.urlMu.Unlock()
		}(m)
	}
	wg.Wait()
}

// pickURL spreads requests over the primary URL and valid mirrors.
func (d *Download) pickURL() string {
	d.urlMu.Lock()
	defer d.urlMu.Unlock()
	if len(d.urls) <= 1 {
		return d.targetURL()
	}
	for i := 0; i < len(d.urls); i++ {
		u := d.urls[int(d.urlSeq.Add(1))%len(d.urls)]
		until, bad := d.badURLs[u]
		if !bad || time.Now().After(until) {
			return u
		}
	}
	return d.targetURL()
}

func (d *Download) markBad(u string) {
	d.urlMu.Lock()
	defer d.urlMu.Unlock()
	if len(d.urls) <= 1 || u == d.targetURL() {
		return
	}
	if d.badURLs == nil {
		d.badURLs = map[string]time.Time{}
	}
	d.badURLs[u] = time.Now().Add(60 * time.Second)
}

var contentRangeRe = regexp.MustCompile(`bytes\s+(\d+)-(\d+)/(\d+|\*)`)

// probe sends the first request (Range: bytes=0-), learns size/name/range
// support and keeps the body so the first connection starts immediately.
func (d *Download) probe(ctx context.Context) (io.ReadCloser, error) {
	var lastErr error
	for attempt := 0; attempt <= d.cfg.Retries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(d.cfg.RetryDelay, attempt)); err != nil {
				return nil, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.snap.URL, nil)
		if err != nil {
			return nil, err
		}
		d.setHeaders(req)
		req.Header.Set("Range", "bytes=0-")
		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			he := &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
			if he.Fatal() {
				return nil, he
			}
			lastErr = he
			continue
		}

		d.mu.Lock()
		s := &d.snap
		s.FinalURL = resp.Request.URL.String()
		s.ContentType = resp.Header.Get("Content-Type")
		s.ETag = resp.Header.Get("ETag")
		s.LastModified = resp.Header.Get("Last-Modified")
		s.Size = -1
		s.Resumable = false
		if resp.StatusCode == http.StatusPartialContent {
			if m := contentRangeRe.FindStringSubmatch(resp.Header.Get("Content-Range")); m != nil && m[3] != "*" {
				s.Size, _ = strconv.ParseInt(m[3], 10, 64)
				s.Resumable = s.Size > 0
			}
		} else if resp.ContentLength > 0 {
			s.Size = resp.ContentLength
		}
		if s.Path == "" {
			name := s.FileName
			if name == "" {
				name = FileNameFromResponse(s.FinalURL, resp.Header.Get("Content-Disposition"), s.ContentType)
			} else {
				name = SanitizeFileName(name)
			}
			s.FileName = name
			s.Path = UniquePath(s.Dir, name)
		}
		end := s.Size
		if end <= 0 {
			end = -1
		}
		d.segs = []*Segment{{Start: 0, End: end, Pos: 0}}
		d.mu.Unlock()
		return resp.Body, nil
	}
	return nil, lastErr
}

// blockSize adapts the range request size to the connection speed: about
// 2 s of data, between the configured size (8 MiB) and 64 MiB. Fewer
// requests help on fast, high-latency links.
func (d *Download) blockSize(rate float64) int64 {
	bs := d.cfg.BlockSize
	if bs <= 0 {
		return 0
	}
	if r := int64(rate * 2); r > bs {
		bs = min(r, max(maxBlockSize, d.cfg.BlockSize))
	}
	return bs
}

// openRange requests the next block of seg (at most blockSize bytes) so that
// the keep-alive connection is reused for the following block instead of being
// closed when a split shortens the segment. Returns the body, its start offset
// and the block end (-1 = open).
func (d *Download) openRange(ctx context.Context, seg *Segment, rate float64) (io.ReadCloser, int64, int64, error) {
	d.mu.Lock()
	pos, end := seg.Pos, seg.End
	resumable := d.snap.Resumable
	etag, lm := d.snap.ETag, d.snap.LastModified
	d.mu.Unlock()

	until := int64(-1)
	if resumable && end > 0 {
		until = end
		if bs := d.blockSize(rate); bs > 0 && pos+bs < end {
			until = pos + bs
		}
	}
	target := d.pickURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	d.setHeaders(req)
	if resumable {
		if until > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, until-1))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", pos))
		}
		if target == d.targetURL() {
			if etag != "" && !isWeakETag(etag) {
				req.Header.Set("If-Range", etag)
			} else if lm != "" {
				req.Header.Set("If-Range", lm)
			}
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		d.markBad(target)
		return nil, 0, 0, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if m := contentRangeRe.FindStringSubmatch(resp.Header.Get("Content-Range")); m != nil {
			if got, _ := strconv.ParseInt(m[1], 10, 64); got != pos {
				resp.Body.Close()
				d.markBad(target)
				return nil, 0, 0, fmt.Errorf("plage reçue inattendue (%d au lieu de %d)", got, pos)
			}
		}
		return resp.Body, pos, until, nil
	case http.StatusOK:
		if pos == 0 && !resumable {
			return resp.Body, 0, -1, nil
		}
		resp.Body.Close()
		if target != d.targetURL() {
			d.markBad(target)
			return nil, 0, 0, errors.New("le miroir ne prend pas en charge les plages")
		}
		if pos == 0 {
			return nil, 0, 0, ErrNoRange
		}
		if resumable && (etag != "" || lm != "") {
			return nil, 0, 0, ErrRemoteChanged
		}
		return nil, 0, 0, ErrNoRange
	default:
		resp.Body.Close()
		if target != d.targetURL() {
			d.markBad(target)
			return nil, 0, 0, fmt.Errorf("miroir : HTTP %s", resp.Status) // not fatal: retry on another URL
		}
		return nil, 0, 0, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
}

func isWeakETag(e string) bool { return len(e) > 2 && e[:2] == "W/" }

// workerLoop downloads seg, then keeps claiming new work until none is left.
// wctx is the context of the current segment's requests (cancelled when a
// duplicate request finishes the segment first).
func (d *Download) workerLoop(ctx context.Context, seg *Segment, c *wctl, wctx context.Context, body io.ReadCloser, release func()) {
	defer release()
	var rate float64 // this connection's speed (EWMA)
	for seg != nil {
		err := d.worker(wctx, seg, body, &rate)
		body = nil
		c.cancel()
		d.mu.Lock()
		seg.detach(c)
		finished := seg.complete()
		if finished {
			// first writer wins: stop the duplicate still working on it
			for _, o := range seg.holders {
				o.cancel()
			}
			d.mergeLocked()
		}
		if err != nil && !finished {
			if ctx.Err() == nil {
				d.lastErr = err
				var he *HTTPError
				if errors.Is(err, ErrRemoteChanged) || errors.Is(err, ErrNoRange) || (errors.As(err, &he) && he.Fatal()) {
					d.fatalErr = err
				}
			}
			d.mu.Unlock()
			return
		}
		if ctx.Err() != nil || atomic.LoadInt32(&d.active) > atomic.LoadInt32(&d.maxConns) || d.fatalErr != nil {
			d.mu.Unlock()
			return
		}
		var cancel context.CancelFunc
		wctx, cancel = context.WithCancel(ctx)
		c = &wctl{cancel: cancel}
		seg = d.claimLocked(c, rate)
		d.mu.Unlock()
		if seg == nil {
			cancel()
		}
	}
}

func (d *Download) worker(ctx context.Context, seg *Segment, body io.ReadCloser, rate *float64) error {
	attempt := 0
	buf := make([]byte, d.cfg.BufSize)
	until := int64(-1) // the probe body (if any) runs until the segment end
	var off int64
	if body != nil {
		d.mu.Lock()
		off = seg.Pos
		d.mu.Unlock()
	}
	for {
		if body == nil {
			d.mu.Lock()
			finished := seg.complete()
			d.mu.Unlock()
			if finished {
				return nil
			}
			b, o, u, err := d.openRange(ctx, seg, *rate)
			until = u
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				var he *HTTPError
				if errors.Is(err, ErrRemoteChanged) || errors.Is(err, ErrNoRange) || (errors.As(err, &he) && he.Fatal()) {
					return err
				}
				// connect/header timeout or 429/503: the server is limiting us,
				// use one connection less for this download
				var ne net.Error
				if (errors.As(err, &ne) && ne.Timeout()) || (he != nil && (he.StatusCode == 429 || he.StatusCode == 503)) {
					if mc := atomic.LoadInt32(&d.maxConns); mc > 1 {
						atomic.CompareAndSwapInt32(&d.maxConns, mc, mc-1)
						return err
					}
				}
				attempt++
				if attempt > d.cfg.Retries {
					return err
				}
				if err := sleepCtx(ctx, backoff(d.cfg.RetryDelay, attempt)); err != nil {
					return err
				}
				continue
			}
			body, off = b, o
		}
		start := off
		end, err := d.copy(ctx, seg, body, buf, off, until, rate)
		off = end
		body.Close()
		body = nil
		if err == nil {
			d.mu.Lock()
			more := seg.End < 0 || seg.Pos < seg.End
			d.mu.Unlock()
			if more && seg.End >= 0 {
				continue // block done: request the next block on the same connection
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if end-start >= 1<<20 {
			attempt = 0 // real progress was made: reset retry budget
		}
		// A stalled connection while the server throttles parallel
		// connections: drop one connection for this download and let
		// another worker take the segment over.
		if errors.Is(err, errIdle) {
			if mc := atomic.LoadInt32(&d.maxConns); mc > 1 {
				atomic.CompareAndSwapInt32(&d.maxConns, mc, mc-1)
				return err
			}
		}
		attempt++
		if attempt > d.cfg.Retries {
			return err
		}
		if !d.snap.Resumable {
			// Non-resumable stream broken: restart from zero.
			d.mu.Lock()
			seg.Pos = seg.Start
			d.mu.Unlock()
		}
		if err := sleepCtx(ctx, backoff(d.cfg.RetryDelay, attempt)); err != nil {
			return err
		}
	}
}

// wdReader resets an idle watchdog timer each time data arrives.
type wdReader struct {
	r io.Reader
	t *time.Timer
	d time.Duration
}

func (w *wdReader) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.t.Reset(w.d)
	}
	return n, err
}

var errIdle = errors.New("délai d'attente dépassé (aucune donnée reçue)")

// waitLimit applies the speed limits before data is written.
func (d *Download) waitLimit(ctx context.Context, n int) error {
	d.mu.Lock()
	sh := d.share
	d.mu.Unlock()
	if sh != nil {
		if err := sh.WaitN(ctx, n); err != nil {
			return err
		}
	} else if err := d.cfg.SpeedLimiter.WaitN(ctx, n); err != nil {
		return err
	}
	return d.Limiter.WaitN(ctx, n)
}

// copy streams body into the file from offset off until the segment end
// (which may shrink concurrently when another connection steals half of it)
// or until the end of the requested block (until >= 0). Each worker writes at
// its own offset: with a duplicate request, both write identical bytes and
// seg.Pos is the furthest position reached. Returns the offset reached.
func (d *Download) copy(ctx context.Context, seg *Segment, body io.ReadCloser, buf []byte, off, until int64, rate *float64) (int64, error) {
	var idle atomic.Bool
	wd := time.AfterFunc(d.cfg.Timeout, func() { idle.Store(true); body.Close() })
	defer wd.Stop()
	rd := &wdReader{r: body, t: wd, d: d.cfg.Timeout}
	// limit returns where writing must stop (segment end or block end), under d.mu.
	limit := func() int64 {
		end := seg.End
		if until >= 0 && (end < 0 || until < end) {
			return until
		}
		return end
	}
	minChunk := min(256<<10, len(buf))
	winStart, winBytes := time.Now(), int64(0)
	for {
		if ctx.Err() != nil {
			return off, ctx.Err()
		}
		n0, rerr := io.ReadAtLeast(rd, buf, minChunk)
		if idle.Load() {
			return off, errIdle
		}
		if n0 > 0 {
			d.mu.Lock()
			end := limit()
			done := seg.complete()
			d.mu.Unlock()
			n := int64(n0)
			if end >= 0 && off+n > end {
				n = end - off
			}
			if n <= 0 || done {
				return off, nil // segment shrunk by a split, or finished by a duplicate
			}
			if err := d.waitLimit(ctx, int(n)); err != nil {
				return off, err
			}
			if _, err := d.file.WriteAt(buf[:n], off); err != nil {
				return off, err
			}
			off += n
			winBytes += n
			now := time.Now()
			d.mu.Lock()
			if off > seg.Pos {
				seg.Pos = off
				if seg.End >= 0 && seg.Pos > seg.End {
					seg.Pos = seg.End // a split moved End while we were writing
				}
			}
			seg.lastWrite = now
			lim := limit()
			reached := (lim >= 0 && off >= lim) || seg.complete()
			if el := now.Sub(winStart); el >= 500*time.Millisecond || (reached && el > 0 && winBytes >= 64<<10) {
				d.updateRateLocked(seg, rate, float64(winBytes)/el.Seconds())
				winStart, winBytes = now, 0
			}
			d.mu.Unlock()
			if reached {
				return off, nil
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF) {
				d.mu.Lock()
				defer d.mu.Unlock()
				if seg.End < 0 {
					seg.End = off
					seg.Pos = off
					if d.snap.Size <= 0 {
						d.snap.Size = off
					}
					return off, nil
				}
				if lim := limit(); off >= lim || seg.complete() {
					return off, nil // end of the segment or of the requested block
				}
				return off, io.ErrUnexpectedEOF
			}
			return off, rerr
		}
	}
}

// updateRateLocked folds a speed measurement into the worker's rate, its
// segment's rate and the download's typical connection speed.
func (d *Download) updateRateLocked(seg *Segment, rate *float64, inst float64) {
	if *rate == 0 {
		*rate = inst
	} else {
		*rate = 0.6**rate + 0.4*inst
	}
	if len(seg.holders) <= 1 || *rate > seg.rate {
		seg.rate = *rate
	}
	if d.connRates == nil || len(d.connRates) > 64 {
		d.connRates = map[*float64]float64{}
	}
	d.connRates[rate] = *rate
}

// backoff returns the delay before retry attempt (exponential, ±30 % jitter so
// that the connections of a download do not retry in synchronized bursts).
func backoff(base time.Duration, attempt int) time.Duration {
	d := base << uint(min(attempt-1, 6))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return time.Duration(float64(d) * (0.7 + 0.6*rand.Float64()))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
