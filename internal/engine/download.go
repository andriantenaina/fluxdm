package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// Segment is a byte range [Start, End) of the file; Pos is the next byte to write.
// End = -1 means "until EOF" (unknown size).
type Segment struct {
	Start int64 `json:"s"`
	End   int64 `json:"e"`
	Pos   int64 `json:"p"`
	owned bool
}

func (s *Segment) remaining() int64 {
	if s.End < 0 {
		return 1 << 62
	}
	return s.End - s.Pos
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
	OnSave     func(Snapshot) // every ~2 s and on exit, for persistence

	mu       sync.Mutex
	snap     Snapshot
	segs     []*Segment
	file     *os.File
	active   int32
	maxConns int32
	lastErr  error
	fatalErr error
	cancel   context.CancelFunc
	done     chan struct{}

	speed     float64
	lastBytes int64
	lastTick  time.Time
}

// NewDownload creates a download from a (new or saved) snapshot.
func NewDownload(client *http.Client, cfg Config, snap Snapshot) *Download {
	cfg.normalize()
	d := &Download{cfg: cfg, client: client, snap: snap, maxConns: int32(cfg.MaxConns)}
	for i := range snap.Segments {
		s := snap.Segments[i]
		d.segs = append(d.segs, &s)
	}
	return d
}

// SetMaxConns changes the number of connections while running.
func (d *Download) SetMaxConns(n int) {
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	atomic.StoreInt32(&d.maxConns, int32(n))
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
		c := *sg
		c.owned = false
		s.Segments = append(s.Segments, c)
	}
	return s
}

// Run downloads until completion, error, or context cancellation (pause).
func (d *Download) Run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.cancel = cancel
	d.done = make(chan struct{}, 64)

	var firstBody io.ReadCloser
	var firstSeg *Segment
	if d.snap.Completed {
		return nil
	}
	if len(d.segs) == 0 || d.snap.Path == "" {
		body, err := d.probe(ctx)
		if err != nil {
			return err
		}
		firstBody = body
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

	if err := os.MkdirAll(filepath.Dir(d.snap.Path), 0o755); err != nil {
		closeBody(firstBody)
		return err
	}
	f, err := os.OpenFile(d.snap.Path+PartExt, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		closeBody(firstBody)
		return err
	}
	d.file = f
	defer f.Close()
	if d.snap.Size > 0 {
		if st, err := f.Stat(); err == nil && st.Size() != d.snap.Size {
			if err := f.Truncate(d.snap.Size); err != nil {
				closeBody(firstBody)
				return err
			}
		}
	}

	var wg sync.WaitGroup
	start := func(seg *Segment, body io.ReadCloser) {
		atomic.AddInt32(&d.active, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.workerLoop(ctx, seg, body)
			atomic.AddInt32(&d.active, -1)
			d.done <- struct{}{}
		}()
	}

	if firstBody != nil {
		d.mu.Lock()
		firstSeg = d.segs[0]
		firstSeg.owned = true
		d.mu.Unlock()
		start(firstSeg, firstBody)
	}

	d.lastTick = time.Now()
	d.lastBytes = d.downloaded()
	progressT := time.NewTicker(250 * time.Millisecond)
	defer progressT.Stop()
	spawnT := time.NewTicker(150 * time.Millisecond)
	defer spawnT.Stop()
	lastSave := time.Now()

	spawn := func() {
		for atomic.LoadInt32(&d.active) < atomic.LoadInt32(&d.maxConns) {
			d.mu.Lock()
			if d.fatalErr != nil {
				d.mu.Unlock()
				return
			}
			seg := d.claimLocked()
			d.mu.Unlock()
			if seg == nil {
				return
			}
			start(seg, nil)
			// Add connections gradually (one per tick) so splits stay balanced.
			if atomic.LoadInt32(&d.active) > 1 {
				return
			}
		}
	}
	spawn()

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
				if time.Since(lastSave) > 2*time.Second {
					lastSave = time.Now()
					d.save()
				}
			}
		}
	}()

	cancel()
	wg.Wait()
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
	}
	d.save()
	if result != nil && parent.Err() != nil {
		return parent.Err()
	}
	return result
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
		if s.End < 0 || s.Pos < s.End {
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
	for _, s := range d.segs {
		c := *s
		p.Segments = append(p.Segments, c)
	}
	d.mu.Unlock()
	d.OnProgress(p)
}

// claimLocked returns a segment for a free connection: first an unowned
// unfinished one, otherwise the second half of the largest owned segment.
func (d *Download) claimLocked() *Segment {
	for _, s := range d.segs {
		if !s.owned && s.remaining() > 0 {
			s.owned = true
			return s
		}
	}
	if !d.snap.Resumable || d.snap.Size <= 0 {
		return nil
	}
	var best *Segment
	var bestRem int64
	for _, s := range d.segs {
		if s.owned && s.End > 0 {
			if r := s.remaining(); r > bestRem {
				best, bestRem = s, r
			}
		}
	}
	if best == nil || bestRem < 2*d.cfg.MinSplit {
		return nil
	}
	mid := best.Pos + bestRem/2
	ns := &Segment{Start: mid, End: best.End, Pos: mid, owned: true}
	best.End = mid
	d.segs = append(d.segs, ns)
	sort.Slice(d.segs, func(i, j int) bool { return d.segs[i].Start < d.segs[j].Start })
	return ns
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

// openRange requests the remaining bytes of seg.
func (d *Download) openRange(ctx context.Context, seg *Segment) (io.ReadCloser, error) {
	d.mu.Lock()
	pos, end := seg.Pos, seg.End
	resumable := d.snap.Resumable
	etag, lm := d.snap.ETag, d.snap.LastModified
	d.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.targetURL(), nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(req)
	if resumable {
		if end > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, end-1))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", pos))
		}
		if etag != "" && !isWeakETag(etag) {
			req.Header.Set("If-Range", etag)
		} else if lm != "" {
			req.Header.Set("If-Range", lm)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if m := contentRangeRe.FindStringSubmatch(resp.Header.Get("Content-Range")); m != nil {
			if got, _ := strconv.ParseInt(m[1], 10, 64); got != pos {
				resp.Body.Close()
				return nil, fmt.Errorf("plage reçue inattendue (%d au lieu de %d)", got, pos)
			}
		}
		return resp.Body, nil
	case http.StatusOK:
		if pos == 0 {
			return resp.Body, nil
		}
		resp.Body.Close()
		if resumable && (etag != "" || lm != "") {
			return nil, ErrRemoteChanged
		}
		return nil, ErrNoRange
	default:
		resp.Body.Close()
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
}

func isWeakETag(e string) bool { return len(e) > 2 && e[:2] == "W/" }

// workerLoop downloads seg, then keeps claiming new work until none is left.
func (d *Download) workerLoop(ctx context.Context, seg *Segment, body io.ReadCloser) {
	for seg != nil {
		err := d.worker(ctx, seg, body)
		body = nil
		d.mu.Lock()
		seg.owned = false
		if err != nil {
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
		if atomic.LoadInt32(&d.active) > atomic.LoadInt32(&d.maxConns) {
			d.mu.Unlock()
			return
		}
		seg = d.claimLocked()
		d.mu.Unlock()
	}
}

func (d *Download) worker(ctx context.Context, seg *Segment, body io.ReadCloser) error {
	attempt := 0
	buf := make([]byte, d.cfg.BufSize)
	for {
		if body == nil {
			b, err := d.openRange(ctx, seg)
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
			body = b
		}
		before := seg.Pos
		err := d.copy(ctx, seg, body, buf)
		body.Close()
		body = nil
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		d.mu.Lock()
		progressed := seg.Pos - before
		d.mu.Unlock()
		if progressed >= 1<<20 {
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

// copy streams body into the file at seg.Pos until the segment end (which may
// shrink concurrently when another connection steals half of it).
func (d *Download) copy(ctx context.Context, seg *Segment, body io.ReadCloser, buf []byte) error {
	var idle atomic.Bool
	wd := time.AfterFunc(d.cfg.Timeout, func() { idle.Store(true); body.Close() })
	defer wd.Stop()
	rd := &wdReader{r: body, t: wd, d: d.cfg.Timeout}
	minChunk := 64 << 10
	if minChunk > len(buf) {
		minChunk = len(buf)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n0, rerr := io.ReadAtLeast(rd, buf, minChunk)
		if idle.Load() {
			return errIdle
		}
		r := struct {
			n   int
			err error
		}{n0, rerr}
		if r.n > 0 {
			d.mu.Lock()
			pos, end := seg.Pos, seg.End
			d.mu.Unlock()
			n := int64(r.n)
			if end >= 0 && pos+n > end {
				n = end - pos
			}
			if n > 0 {
				if _, err := d.file.WriteAt(buf[:n], pos); err != nil {
					return err
				}
				d.mu.Lock()
				seg.Pos = pos + n
				reached := seg.End >= 0 && seg.Pos >= seg.End
				d.mu.Unlock()
				if err := d.cfg.SpeedLimiter.WaitN(ctx, int(n)); err != nil {
					return err
				}
				if err := d.Limiter.WaitN(ctx, int(n)); err != nil {
					return err
				}
				if reached {
					return nil
				}
			} else {
				return nil // segment shrunk by a split and is complete
			}
		}
		if r.err != nil {
			if errors.Is(r.err, io.EOF) || errors.Is(r.err, io.ErrUnexpectedEOF) {
				d.mu.Lock()
				defer d.mu.Unlock()
				if seg.End < 0 {
					seg.End = seg.Pos
					if d.snap.Size <= 0 {
						d.snap.Size = seg.Pos
					}
					return nil
				}
				if seg.Pos >= seg.End {
					return nil
				}
				return io.ErrUnexpectedEOF
			}
			return r.err
		}
	}
}

func backoff(base time.Duration, attempt int) time.Duration {
	d := base << uint(min(attempt-1, 6))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
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
