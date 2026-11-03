package media

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
)

// Track is a list of segments forming one elementary stream (video, audio or muxed).
type Track struct {
	Name string   `json:"name"` // main | video | audio
	Init *SegRef  `json:"init,omitempty"`
	Segs []SegRef `json:"segs"`
	FMP4 bool     `json:"fmp4"`
}

// trackState is persisted for resume: Done[i] = bytes of segment i (0 = missing).
type trackState struct {
	Name string  `json:"name"`
	Done []int64 `json:"done"`
}

// fetcher downloads small resources (segments, keys, playlists) with the item headers.
type fetcher struct {
	client  *http.Client
	headers map[string]string
	ua      string
	limiter *engine.Limiter
	keys    sync.Map // uri -> []byte
}

func newFetcher(m *core.Manager, headers map[string]string) *fetcher {
	cfg := m.EngineConfig()
	return &fetcher{client: m.HTTPClient(), headers: headers, ua: cfg.UserAgent, limiter: m.Limiter()}
}

func (f *fetcher) request(ctx context.Context, rawURL string, br *ByteRange) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range f.headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", f.ua)
	}
	if br != nil {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", br.Offset, br.Offset+br.Length-1))
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, &engine.HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return resp, nil
}

// get fetches a whole resource into memory (segments are small).
func (f *fetcher) get(ctx context.Context, rawURL string, br *ByteRange) ([]byte, error) {
	resp, err := f.request(ctx, rawURL, br)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

// getText fetches a playlist/manifest and returns body + final URL.
func (f *fetcher) getText(ctx context.Context, rawURL string) (string, string, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, time.Duration(attempt)*time.Second); err != nil {
				return "", "", err
			}
		}
		resp, err := f.request(ctx, rawURL, nil)
		if err != nil {
			lastErr = err
			var he *engine.HTTPError
			if errors.As(err, &he) && he.Fatal() {
				return "", "", err
			}
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return string(b), resp.Request.URL.String(), nil
	}
	return "", "", lastErr
}

func (f *fetcher) key(ctx context.Context, k *Key) ([]byte, error) {
	if v, ok := f.keys.Load(k.URI); ok {
		return v.([]byte), nil
	}
	var b []byte
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		b, err = f.get(ctx, k.URI, nil)
		if err == nil {
			break
		}
		if e := sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond); e != nil {
			return nil, e
		}
	}
	if err != nil {
		return nil, fmt.Errorf("clé de chiffrement : %w", err)
	}
	if len(b) != 16 {
		// Some servers return the key hex-encoded or base64; try hex.
		s := strings.TrimSpace(string(b))
		if len(s) == 32 {
			var hb [16]byte
			if _, e := fmt.Sscanf(s, "%32x", &hb); e == nil {
				b = hb[:]
			}
		}
		if len(b) != 16 {
			return nil, fmt.Errorf("clé AES invalide (%d octets)", len(b))
		}
	}
	f.keys.Store(k.URI, b)
	return b, nil
}

func decryptAES128(data, key, iv []byte) ([]byte, error) {
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("segment chiffré de taille invalide")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	if n := len(out); n > 0 {
		pad := int(out[n-1])
		if pad > 0 && pad <= aes.BlockSize && pad <= n && bytes.Equal(out[n-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
			out = out[:n-pad]
		}
	}
	return out, nil
}

func seqIV(seq int64) []byte {
	iv := make([]byte, 16)
	binary.BigEndian.PutUint64(iv[8:], uint64(seq))
	return iv
}

// fixTS strips fake headers that some hosts prepend to MPEG-TS segments
// (e.g. a PNG/JPEG header to disguise video as images).
func fixTS(b []byte) []byte {
	if len(b) > 0 && b[0] == 0x47 && (len(b) < 189 || b[188] == 0x47) {
		return b
	}
	limit := len(b) - 376
	if limit > 64<<10 {
		limit = 64 << 10
	}
	for i := 0; i < limit; i++ {
		if b[i] == 0x47 && b[i+188] == 0x47 && b[i+376] == 0x47 {
			return b[i:]
		}
	}
	return b
}

// streamJob downloads all tracks concurrently with resume support.
type streamJob struct {
	f       *fetcher
	tmp     string
	tracks  []Track
	states  []trackState
	workers int
	save    func()

	mu       sync.Mutex
	bytes    atomic.Int64
	segsDone atomic.Int64
	total    int64
}

func (j *streamJob) segPath(ti, si int) string {
	return filepath.Join(j.tmp, j.tracks[ti].Name, fmt.Sprintf("%06d.seg", si))
}

func (j *streamJob) initPath(ti int) string {
	return filepath.Join(j.tmp, j.tracks[ti].Name, "init.seg")
}

// prepare aligns persisted state with the current track list.
func (j *streamJob) prepare() error {
	if len(j.states) != len(j.tracks) {
		j.states = make([]trackState, len(j.tracks))
	}
	for ti, t := range j.tracks {
		st := &j.states[ti]
		if st.Name != t.Name || len(st.Done) != len(t.Segs) {
			st.Name = t.Name
			old := st.Done
			st.Done = make([]int64, len(t.Segs))
			copy(st.Done, old)
		}
		if err := os.MkdirAll(filepath.Join(j.tmp, t.Name), 0o755); err != nil {
			return err
		}
		for si, n := range st.Done {
			if n > 0 {
				if fi, err := os.Stat(j.segPath(ti, si)); err != nil || fi.Size() != n {
					st.Done[si] = 0
				} else {
					j.bytes.Add(n)
					j.segsDone.Add(1)
				}
			}
		}
		j.total += int64(len(t.Segs))
	}
	return nil
}

func (j *streamJob) run(ctx context.Context) error {
	if err := j.prepare(); err != nil {
		return err
	}
	// init segments (fMP4)
	for ti, t := range j.tracks {
		if t.Init == nil {
			continue
		}
		if _, err := os.Stat(j.initPath(ti)); err == nil {
			continue
		}
		b, err := j.fetchSeg(ctx, *t.Init, false)
		if err != nil {
			return fmt.Errorf("segment d'initialisation : %w", err)
		}
		if err := os.WriteFile(j.initPath(ti), b, 0o644); err != nil {
			return err
		}
	}

	type job struct{ ti, si int }
	jobs := make(chan job)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var firstErr error
	var errOnce sync.Once
	var wg sync.WaitGroup
	n := j.workers
	if n <= 0 {
		n = 8
	}
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for jb := range jobs {
				if err := j.downloadSeg(ctx, jb.ti, jb.si); err != nil {
					errOnce.Do(func() { firstErr = err; cancel() })
					return
				}
			}
		}()
	}
	lastSave := time.Now()
feed:
	for ti, t := range j.tracks {
		for si := range t.Segs {
			j.mu.Lock()
			done := j.states[ti].Done[si] > 0
			j.mu.Unlock()
			if done {
				continue
			}
			select {
			case jobs <- job{ti, si}:
			case <-ctx.Done():
				break feed
			}
			if time.Since(lastSave) > 3*time.Second {
				lastSave = time.Now()
				j.save()
			}
		}
	}
	close(jobs)
	wg.Wait()
	j.save()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (j *streamJob) fetchSeg(ctx context.Context, s SegRef, isTS bool) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			d := time.Duration(1<<min(attempt-1, 5)) * 400 * time.Millisecond
			if err := sleep(ctx, d); err != nil {
				return nil, err
			}
		}
		b, err := j.f.get(ctx, s.URL, s.Range)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			var he *engine.HTTPError
			if errors.As(err, &he) && (he.StatusCode == 401 || he.StatusCode == 403 || he.StatusCode == 410) && attempt >= 3 {
				return nil, fmt.Errorf("accès refusé au segment (lien expiré ?) : %w", err)
			}
			continue
		}
		if s.Key != nil && s.Key.Method == "AES-128" {
			key, err := j.f.key(ctx, s.Key)
			if err != nil {
				return nil, err
			}
			iv := s.Key.IV
			if len(iv) != 16 {
				iv = seqIV(s.Seq)
			}
			if b, err = decryptAES128(b, key, iv); err != nil {
				lastErr = err
				continue
			}
		}
		if isTS {
			b = fixTS(b)
		}
		if err := j.f.limiter.WaitN(ctx, len(b)); err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, lastErr
}

func (j *streamJob) downloadSeg(ctx context.Context, ti, si int) error {
	t := j.tracks[ti]
	b, err := j.fetchSeg(ctx, t.Segs[si], !t.FMP4)
	if err != nil {
		return err
	}
	p := j.segPath(ti, si)
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(p+".tmp", p); err != nil {
		return err
	}
	j.mu.Lock()
	j.states[ti].Done[si] = int64(len(b))
	j.mu.Unlock()
	j.bytes.Add(int64(len(b)))
	j.segsDone.Add(1)
	return nil
}

// estimate returns downloaded bytes and an estimated total size.
func (j *streamJob) estimate() (int64, int64) {
	done := j.bytes.Load()
	n := j.segsDone.Load()
	if n == 0 || j.total == 0 {
		return done, -1
	}
	return done, done * j.total / n
}

// concatTrack joins the segments of a track into one file.
func (j *streamJob) concatTrack(ti int, out string) error {
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	defer w.Close()
	if j.tracks[ti].Init != nil {
		if err := appendFile(w, j.initPath(ti)); err != nil {
			return err
		}
	}
	written := 0
	for si := range j.tracks[ti].Segs {
		if si < len(j.states[ti].Done) && j.states[ti].Done[si] == 0 {
			continue // not downloaded (stopped live recording)
		}
		if err := appendFile(w, j.segPath(ti, si)); err != nil {
			return err
		}
		written++
	}
	if written == 0 {
		return errors.New("aucun segment téléchargé")
	}
	return w.Sync()
}

func appendFile(w io.Writer, p string) error {
	r, err := os.Open(p)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	return err
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
