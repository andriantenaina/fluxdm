package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A server where the first request covering one offset trickles at ~32 KiB/s
// while every other request is fast. Work stealing then the endgame duplicate
// request must finish the download quickly and correctly.
func TestEndgameStraggler(t *testing.T) {
	data := randomData(t, 8<<20)
	slowAt := int64(5<<20 + 12345)
	var slowUsed atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, end := int64(0), int64(len(data)-1)
		if rg := r.Header.Get("Range"); rg != "" {
			var s, e int64 = 0, -1
			if n, _ := fmt.Sscanf(rg, "bytes=%d-%d", &s, &e); n >= 1 {
				start = s
				if n == 2 && e >= 0 && e < end {
					end = e
				}
			}
		}
		slow := start <= slowAt && slowAt <= end && slowUsed.CompareAndSwap(false, true)
		w.Header().Set("ETag", `"eg"`)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		chunk := int64(64 << 10)
		if slow {
			chunk = 8 << 10
		}
		for p := start; p <= end; p += chunk {
			q := min(p+chunk, end+1)
			if _, err := w.Write(data[p:q]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			if slow {
				time.Sleep(250 * time.Millisecond)
			}
		}
	}))
	defer srv.Close()
	cfg := testConfig()
	cfg.MaxConns = 4
	cfg.AutoConns = false
	cfg.MinSplit = 1 << 20 // stealing stops at 256 KiB halves: the tail needs the endgame
	start := time.Now()
	// non-browser User-Agent: some security software buffers slow loopback
	// responses sent to browsers, which would hide what is measured here
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/eg.bin", Dir: t.TempDir(), Headers: map[string]string{"User-Agent": "fluxdm-test"}})
	if err != nil {
		t.Fatal(err)
	}
	el := time.Since(start)
	checkFile(t, d.Snapshot().Path, data)
	t.Logf("terminé en %v, requêtes dupliquées : %d", el, d.dups)
	// Without stealing the slow request would need > 2 min; without the
	// endgame the last <512 KiB alone would take up to 16 s.
	if el > 3*time.Second*raceSlowdown {
		t.Errorf("fin de téléchargement trop lente : %v", el)
	}
	if n := len(d.Snapshot().Segments); n != 1 {
		t.Errorf("segments terminés non fusionnés : %d", n)
	}
}

func TestMergeSegments(t *testing.T) {
	d := &Download{}
	busy := &Segment{Start: 300, End: 400, Pos: 350, holders: []*wctl{{}}}
	d.segs = []*Segment{
		{Start: 0, End: 100, Pos: 100},
		{Start: 100, End: 200, Pos: 200},
		{Start: 200, End: 300, Pos: 250}, // unfinished
		busy,
		{Start: 400, End: 500, Pos: 500},
		{Start: 500, End: 600, Pos: 600},
		{Start: 600, End: 700, Pos: 700},
	}
	d.mergeLocked()
	want := [][3]int64{{0, 200, 200}, {200, 300, 250}, {300, 400, 350}, {400, 700, 700}}
	if len(d.segs) != len(want) {
		t.Fatalf("%d segments, attendu %d", len(d.segs), len(want))
	}
	for i, w := range want {
		s := d.segs[i]
		if s.Start != w[0] || s.End != w[1] || s.Pos != w[2] {
			t.Errorf("segment %d = %d-%d@%d, attendu %v", i, s.Start, s.End, s.Pos, w)
		}
	}
	if d.segs[2] != busy {
		t.Error("un segment en cours a été remplacé")
	}
	// sorted insertion after a split
	d.insertAfterLocked(d.segs[1], &Segment{Start: 260, End: 300, Pos: 260})
	for i := 1; i < len(d.segs); i++ {
		if d.segs[i-1].Start >= d.segs[i].Start {
			t.Fatalf("liste non triée : %v", d.segs)
		}
	}
}

// Two downloads sharing a gate of 3 connections per host: the server must
// never see more than 3 simultaneous requests.
func TestHostGate(t *testing.T) {
	data := randomData(t, 3<<20)
	var active, maxActive int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&active, 1)
		// stop counting as soon as the client closes the connection, not when
		// this handler next notices it
		var once sync.Once
		dec := func() { once.Do(func() { atomic.AddInt32(&active, -1) }) }
		defer dec()
		stop := context.AfterFunc(r.Context(), dec)
		defer stop()
		for {
			m := atomic.LoadInt32(&maxActive)
			if n <= m || atomic.CompareAndSwapInt32(&maxActive, m, n) {
				break
			}
		}
		start, end := int64(0), int64(len(data)-1)
		if rg := r.Header.Get("Range"); rg != "" {
			var s, e int64 = 0, -1
			if k, _ := fmt.Sscanf(rg, "bytes=%d-%d", &s, &e); k >= 1 {
				start = s
				if k == 2 && e >= 0 && e < end {
					end = e
				}
			}
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		for p := start; p <= end; p += 16 << 10 {
			select {
			case <-r.Context().Done(): // the client went away: stop counting it
				return
			default:
			}
			if _, err := w.Write(data[p:min(p+16<<10, end+1)]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(time.Millisecond)
		}
	}))
	defer srv.Close()
	cfg := testConfig()
	cfg.MaxConns = 4
	cfg.HostGate = NewHostGate(3)
	cl, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var cActive, cMax int32
	cl.Transport = countingRT{rt: cl.Transport, active: &cActive, max: &cMax}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	paths := make([]string, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := NewDownload(cl, cfg, Snapshot{URL: srv.URL + "/g" + strconv.Itoa(i), Dir: t.TempDir()})
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			errs[i] = d.Run(ctx)
			paths[i] = d.Snapshot().Path
		}(i)
	}
	wg.Wait()
	for i := range 2 {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		checkFile(t, paths[i], data)
	}
	if m := atomic.LoadInt32(&cMax); m > 3 {
		t.Errorf("%d connexions simultanées vers l'hôte, limite 3", m)
	} else {
		t.Logf("pic de connexions : %d", m)
	}
	if n := cfg.HostGate.InUse("127.0.0.1"); n != 0 {
		t.Errorf("%d emplacements non libérés", n)
	}
}

// Two saturating consumers of a 4 MiB/s limit get the same share (±10 %),
// whatever their number of goroutines (connections).
func TestFairLimiter(t *testing.T) {
	l := NewLimiter(4 << 20)
	a, b := l.NewShare(1), l.NewShare(1)
	defer a.Close()
	defer b.Close()
	var got [2]atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	consume := func(s *Share, i, conns int) {
		for range conns {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for s.WaitN(ctx, 32<<10) == nil {
					got[i].Add(32 << 10)
				}
			}()
		}
	}
	consume(a, 0, 8) // 8 "connections"
	consume(b, 1, 1) // 1 "connection"
	wg.Wait()
	x, y := float64(got[0].Load()), float64(got[1].Load())
	ratio := x / y
	t.Logf("A=%.0f KiB B=%.0f KiB total=%.1f MiB/s ratio=%.2f", x/1024, y/1024, (x+y)/2/(1<<20), ratio)
	if ratio < 0.9 || ratio > 1.1 {
		t.Errorf("partage inéquitable : ratio %.2f", ratio)
	}
	if total := (x + y) / 2; total > 4.6*(1<<20) {
		t.Errorf("limite dépassée : %.1f MiB/s", total/(1<<20))
	}

	// raising the rate wakes waiters immediately
	slow := NewLimiter(1 << 10)
	s := slow.NewShare(1)
	done := make(chan struct{})
	go func() { _ = s.WaitN(context.Background(), 64<<10); close(done) }()
	time.Sleep(50 * time.Millisecond)
	slow.SetRate(0)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("SetRate n'a pas réveillé l'attente")
	}
}

func TestFairLimiterWeights(t *testing.T) {
	l := NewLimiter(4 << 20)
	hi, lo := l.NewShare(2), l.NewShare(1)
	defer hi.Close()
	defer lo.Close()
	var got [2]atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i, s := range []*Share{hi, lo} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s.WaitN(ctx, 32<<10) == nil {
				got[i].Add(32 << 10)
			}
		}()
	}
	wg.Wait()
	ratio := float64(got[0].Load()) / float64(got[1].Load())
	if ratio < 1.7 || ratio > 2.3 {
		t.Errorf("pondération 2:1 non respectée : %.2f", ratio)
	}
}

// countingRT measures client-side concurrency: a request counts from the
// moment it is sent until its response body is closed, which is exactly what
// the host gate controls (a server only notices a close later).
type countingRT struct {
	rt          http.RoundTripper
	active, max *int32
}

func (c countingRT) RoundTrip(r *http.Request) (*http.Response, error) {
	n := atomic.AddInt32(c.active, 1)
	for m := atomic.LoadInt32(c.max); n > m && !atomic.CompareAndSwapInt32(c.max, m, n); m = atomic.LoadInt32(c.max) {
	}
	resp, err := c.rt.RoundTrip(r)
	if err != nil {
		atomic.AddInt32(c.active, -1)
		return nil, err
	}
	var once sync.Once
	resp.Body = countingBody{resp.Body, func() { once.Do(func() { atomic.AddInt32(c.active, -1) }) }}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	done func()
}

func (b countingBody) Close() error { err := b.ReadCloser.Close(); b.done(); return err }
