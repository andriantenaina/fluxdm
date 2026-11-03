package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func randomData(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func testConfig() Config {
	c := DefaultConfig()
	c.MinSplit = 64 << 10
	c.BufSize = 32 << 10
	c.RetryDelay = 20 * time.Millisecond
	c.Timeout = 5 * time.Second
	return c
}

// rangeServer serves data with Range support; optional per-request throttle.
func rangeServer(data []byte, delayPerChunk time.Duration, active *int32, maxActive *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if active != nil {
			n := atomic.AddInt32(active, 1)
			defer atomic.AddInt32(active, -1)
			for {
				m := atomic.LoadInt32(maxActive)
				if n <= m || atomic.CompareAndSwapInt32(maxActive, m, n) {
					break
				}
			}
		}
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Content-Disposition", `attachment; filename="test file.bin"`)
		if delayPerChunk == 0 {
			http.ServeContent(w, r, "f.bin", time.Unix(1700000000, 0), bytes.NewReader(data))
			return
		}
		// slow path: emulate a per-connection throttled server
		start, end := int64(0), int64(len(data)-1)
		if rg := r.Header.Get("Range"); rg != "" {
			var s, e int64 = 0, -1
			if n, _ := fmt.Sscanf(rg, "bytes=%d-%d", &s, &e); n >= 1 {
				start = s
				if n == 2 && e >= 0 && e < end {
					end = e
				}
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		for p := start; p <= end; p += 16 << 10 {
			q := min(p+16<<10, end+1)
			if _, err := w.Write(data[p:q]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(delayPerChunk)
		}
	}))
}

func run(t *testing.T, cfg Config, snap Snapshot) (*Download, error) {
	t.Helper()
	cl, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDownload(cl, cfg, snap)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return d, d.Run(ctx)
}

func checkFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatalf("contenu différent : %d octets reçus, %d attendus", len(got), len(want))
	}
}

func TestMultiConnection(t *testing.T) {
	data := randomData(t, 8<<20+12345)
	var active, maxActive int32
	srv := rangeServer(data, 2*time.Millisecond, &active, &maxActive)
	defer srv.Close()
	dir := t.TempDir()
	cfg := testConfig()
	cfg.MaxConns = 8
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/dl", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Snapshot()
	if filepath.Base(s.Path) != "test file.bin" {
		t.Errorf("nom = %q, attendu le nom de Content-Disposition", filepath.Base(s.Path))
	}
	if !s.Resumable || s.Size != int64(len(data)) {
		t.Errorf("resumable=%v size=%d", s.Resumable, s.Size)
	}
	checkFile(t, s.Path, data)
	if maxActive < 4 {
		t.Errorf("seulement %d connexions simultanées (segmentation dynamique inefficace)", maxActive)
	}
	if _, err := os.Stat(s.Path + PartExt); err == nil {
		t.Error("le fichier .fdmpart n'a pas été renommé")
	}
}

func TestNoRangeServer(t *testing.T) {
	data := randomData(t, 3<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data))) // ignores Range
		w.Write(data)
	}))
	defer srv.Close()
	d, err := run(t, testConfig(), Snapshot{URL: srv.URL + "/file.zip", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Snapshot()
	if s.Resumable {
		t.Error("ne devrait pas être marqué comme reprenable")
	}
	checkFile(t, s.Path, data)
}

func TestUnknownSize(t *testing.T) {
	data := randomData(t, 1<<20+7)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		f := w.(http.Flusher)
		for i := 0; i < len(data); i += 100000 { // chunked, no Content-Length
			w.Write(data[i:min(i+100000, len(data))])
			f.Flush()
		}
	}))
	defer srv.Close()
	d, err := run(t, testConfig(), Snapshot{URL: srv.URL + "/stream", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Snapshot()
	if filepath.Ext(s.Path) != ".zip" {
		t.Errorf("extension depuis le type MIME attendue, chemin = %s", s.Path)
	}
	checkFile(t, s.Path, data)
}

func TestResumeAfterPause(t *testing.T) {
	data := randomData(t, 6<<20)
	srv := rangeServer(data, 3*time.Millisecond, nil, nil)
	defer srv.Close()
	dir := t.TempDir()
	cfg := testConfig()
	cl, _ := NewHTTPClient(cfg)
	d := NewDownload(cl, cfg, Snapshot{URL: srv.URL + "/big", Dir: dir})
	var saved Snapshot
	d.OnSave = func(s Snapshot) { saved = s }
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel() // pause
	}()
	if err := d.Run(ctx); err == nil {
		t.Skip("téléchargement terminé avant la pause (machine trop rapide)")
	}
	var done int64
	for _, sg := range saved.Segments {
		done += sg.Pos - sg.Start
	}
	if done == 0 || done >= int64(len(data)) {
		t.Fatalf("progression sauvegardée invalide : %d", done)
	}
	// resume from the persisted snapshot (as after an app restart)
	d2, err := run(t, cfg, saved)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, d2.Snapshot().Path, data)
}

func TestFlakyConnections(t *testing.T) {
	data := randomData(t, 4<<20)
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&n, 1)
		if c%3 == 0 { // every third request dies after a few bytes
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, buf, _ := hj.Hijack()
				buf.WriteString("HTTP/1.1 206 Partial Content\r\nContent-Length: 999999\r\nContent-Range: bytes 0-999998/" + strconv.Itoa(len(data)) + "\r\n\r\nxx")
				buf.Flush()
				conn.Close()
				return
			}
		}
		http.ServeContent(w, r, "f.bin", time.Unix(1700000000, 0), bytes.NewReader(data))
	}))
	defer srv.Close()
	cfg := testConfig()
	cfg.MaxConns = 6
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/flaky.bin", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, d.Snapshot().Path, data)
}

func TestFatalHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusForbidden)
	}))
	defer srv.Close()
	start := time.Now()
	_, err := run(t, testConfig(), Snapshot{URL: srv.URL + "/x", Dir: t.TempDir()})
	if err == nil {
		t.Fatal("erreur attendue")
	}
	if he, ok := err.(*HTTPError); !ok || he.StatusCode != 403 {
		t.Fatalf("HTTPError 403 attendue, obtenu %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("une erreur 403 ne doit pas être réessayée longtemps")
	}
}

func TestSpeedLimiter(t *testing.T) {
	data := randomData(t, 600<<10)
	srv := rangeServer(data, 0, nil, nil)
	defer srv.Close()
	cfg := testConfig()
	cfg.SpeedLimiter = NewLimiter(300 << 10) // 300 KiB/s
	start := time.Now()
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/lim", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	el := time.Since(start)
	if el < 900*time.Millisecond {
		t.Errorf("limiteur inefficace : %v pour 600 Ko à 300 Ko/s", el)
	}
	checkFile(t, d.Snapshot().Path, data)
}

func TestProbe(t *testing.T) {
	data := randomData(t, 100000)
	srv := rangeServer(data, 0, nil, nil)
	defer srv.Close()
	cfg := testConfig()
	cl, _ := NewHTTPClient(cfg)
	info, err := Probe(context.Background(), cl, cfg, srv.URL+"/a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 100000 || !info.Resumable || info.FileName != "test file.bin" {
		t.Fatalf("probe = %+v", info)
	}
}

func TestFileNames(t *testing.T) {
	cases := []struct{ url, cd, ct, want string }{
		{"https://x.com/a/b/archive.zip?x=1", "", "", "archive.zip"},
		{"https://x.com/dl", `attachment; filename="Rapport final.pdf"`, "", "Rapport final.pdf"},
		{"https://x.com/dl", `attachment; filename*=UTF-8''%C3%A9t%C3%A9.mp4`, "", "été.mp4"},
		{"https://x.com/get", "", "video/mp4", "get.mp4"},
		{"https://x.com/a%20b%3A.txt", "", "", "a b_.txt"},
		{"https://x.com/", "", "", "download"},
	}
	for _, c := range cases {
		if got := FileNameFromResponse(c.url, c.cd, c.ct); got != c.want {
			t.Errorf("FileNameFromResponse(%q,%q,%q) = %q, attendu %q", c.url, c.cd, c.ct, got, c.want)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.zip"), nil, 0o644)
	if p := UniquePath(dir, "a.zip"); filepath.Base(p) != "a (1).zip" {
		t.Errorf("UniquePath = %s", p)
	}
}

// A server that stops sending mid-transfer (keeps the socket open) must be
// detected by the idle timeout and the range retried.
func TestStalledConnection(t *testing.T) {
	data := randomData(t, 2<<20)
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) <= 2 {
			// send headers + a little data, then hang
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Accept-Ranges", "bytes")
			var s int64
			fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &s)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", s, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data))-s, 10))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(data[s : s+1000])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(20 * time.Second):
			}
			return
		}
		http.ServeContent(w, r, "f", time.Unix(1, 0), bytes.NewReader(data))
	}))
	defer srv.Close()
	cfg := testConfig()
	cfg.MaxConns = 1
	cfg.Timeout = time.Second
	start := time.Now()
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/s.bin", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("trop lent : %v", time.Since(start))
	}
	checkFile(t, d.Snapshot().Path, data)
}

// Server allowing only 2 parallel connections: extra ones hang after a few
// bytes. The engine must reduce its connections and still finish.
func TestConnectionLimitedServer(t *testing.T) {
	data := randomData(t, 3<<20)
	var active int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&active, 1) > 2 {
			defer atomic.AddInt32(&active, -1)
			var s int64
			fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &s)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", s, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data))-s, 10))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(data[s : s+100])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(30 * time.Second):
			}
			return
		}
		defer atomic.AddInt32(&active, -1)
		http.ServeContent(w, r, "f", time.Unix(1, 0), bytes.NewReader(data))
	}))
	defer srv.Close()
	cfg := testConfig()
	cfg.MaxConns = 8
	cfg.Timeout = time.Second
	start := time.Now()
	d, err := run(t, cfg, Snapshot{URL: srv.URL + "/lim.bin", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 15*time.Second {
		t.Errorf("trop lent : %v", el)
	}
	checkFile(t, d.Snapshot().Path, data)
}

// Measures the acceleration on a server throttled per connection (~1 MB/s each).
func TestAcceleration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	data := randomData(t, 6<<20)
	srv := rangeServer(data, 16*time.Millisecond, nil, nil) // 16 KiB / 16 ms ≈ 1 MiB/s per connection
	defer srv.Close()
	elapsed := func(conns int) time.Duration {
		cfg := testConfig()
		cfg.MaxConns = conns
		cfg.MinSplit = 256 << 10
		start := time.Now()
		d, err := run(t, cfg, Snapshot{URL: srv.URL + "/acc.bin", Dir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		checkFile(t, d.Snapshot().Path, data)
		return time.Since(start)
	}
	one, eight := elapsed(1), elapsed(8)
	t.Logf("1 connexion : %v — 8 connexions : %v — accélération x%.1f", one, eight, float64(one)/float64(eight))
	if float64(one)/float64(eight) < 3 {
		t.Errorf("accélération insuffisante : x%.1f", float64(one)/float64(eight))
	}
}

var _ = io.EOF
