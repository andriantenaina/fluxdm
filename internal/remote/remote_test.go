package remote

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"fluxdm/internal/core"
)

func newTestServer(t *testing.T) (*Server, *core.Manager, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	m, err := core.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	s := m.Settings()
	s.DownloadDir = filepath.Join(dir, "dl")
	if err := m.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head><body></body></html>")}}
	srv := New(m, nil, assets, []byte("png"), "test")
	hs := httptest.NewServer(srv.routes())
	t.Cleanup(hs.Close)
	return srv, m, hs
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// pairDevice runs the pairing flow and returns the device token.
func pairDevice(t *testing.T, srv *Server, hs *httptest.Server) string {
	t.Helper()
	code, _ := srv.NewPairing()
	resp, err := noRedirect().Get(hs.URL + "/pair?code=" + code)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("pair status = %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	tok := loc.Query().Get("k")
	if len(tok) != 64 {
		t.Fatalf("jeton absent de la redirection : %q", resp.Header.Get("Location"))
	}
	return tok
}

func get(t *testing.T, u, token string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPairingAuthAndRevoke(t *testing.T) {
	srv, m, hs := newTestServer(t)

	if r := get(t, hs.URL+"/api/items", "", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sans jeton : %d", r.StatusCode)
	}
	if r := get(t, hs.URL+"/api/items", strings.Repeat("a", 64), nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("faux jeton : %d", r.StatusCode)
	}

	tok := pairDevice(t, srv, hs)
	devs := m.Settings().Remote.Devices
	if len(devs) != 1 || devs[0].TokenHash == tok || devs[0].TokenHash != hashToken(tok) {
		t.Fatalf("appareil mal enregistré : %+v", devs)
	}
	if r := get(t, hs.URL+"/api/items", tok, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("avec jeton : %d", r.StatusCode)
	}
	// query token (home-screen app, links) works too
	if r := get(t, hs.URL+"/api/items?token="+tok, "", nil); r.StatusCode != http.StatusOK {
		t.Fatalf("jeton en paramètre : %d", r.StatusCode)
	}
	// a stale cookie must not hide a valid header token
	if r := get(t, hs.URL+"/api/items", tok, map[string]string{"Cookie": "fdm_token=" + strings.Repeat("b", 64)}); r.StatusCode != http.StatusOK {
		t.Fatalf("cookie périmé + en-tête valide : %d", r.StatusCode)
	}

	// the manifest of a paired device starts with its token
	r := get(t, hs.URL+"/manifest.webmanifest?k="+tok, "", nil)
	var mf map[string]any
	_ = json.NewDecoder(r.Body).Decode(&mf)
	r.Body.Close()
	if su, _ := mf["start_url"].(string); !strings.Contains(su, tok) {
		t.Fatalf("start_url = %q", su)
	}
	r = get(t, hs.URL+"/manifest.webmanifest?k="+strings.Repeat("c", 64), "", nil)
	mf = nil
	_ = json.NewDecoder(r.Body).Decode(&mf)
	r.Body.Close()
	if su, _ := mf["start_url"].(string); su != "/#/remote" {
		t.Fatalf("start_url avec faux jeton = %q", su)
	}

	if err := srv.Revoke(devs[0].ID); err != nil {
		t.Fatal(err)
	}
	if r := get(t, hs.URL+"/api/items", tok, nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("après révocation : %d", r.StatusCode)
	}
}

func TestPairingCodeOneTimeAndExpired(t *testing.T) {
	srv, _, hs := newTestServer(t)
	code, _ := srv.NewPairing()
	_ = pairWith(t, hs, code, http.StatusFound)
	_ = pairWith(t, hs, code, http.StatusUnauthorized) // already used

	code2, _ := srv.NewPairing()
	srv.mu.Lock()
	srv.codes[code2] = time.Now().Add(-time.Second)
	srv.mu.Unlock()
	_ = pairWith(t, hs, code2, http.StatusUnauthorized)
}

func TestPairingRateLimit(t *testing.T) {
	_, _, hs := newTestServer(t)
	for i := 0; i < 5; i++ {
		pairWith(t, hs, "BADCODE"+string(rune('0'+i)), http.StatusUnauthorized)
	}
	pairWith(t, hs, "BADCODE9", http.StatusTooManyRequests)
}

func pairWith(t *testing.T, hs *httptest.Server, code string, want int) *http.Response {
	t.Helper()
	resp, err := noRedirect().Get(hs.URL + "/pair?code=" + code)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("pair(%s) = %d, attendu %d", code, resp.StatusCode, want)
	}
	return resp
}

func TestFileRangeAndName(t *testing.T) {
	srv, m, hs := newTestServer(t)
	tok := pairDevice(t, srv, hs)

	data := make([]byte, 300<<10)
	_, _ = rand.Read(data)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.mp4", time.Now(), bytes.NewReader(data))
	}))
	defer src.Close()

	var done atomic.Bool
	m.Subscribe(func(ev core.Event) {
		if ev.Type == "completed" {
			done.Store(true)
		}
	})
	it, err := m.Add(core.AddRequest{URL: src.URL + "/v/Mon film.mp4", Start: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for !done.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !done.Load() {
		t.Fatal("téléchargement non terminé")
	}

	if r := get(t, hs.URL+"/files/"+it.ID, "", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/files sans jeton : %d", r.StatusCode)
	}
	r := get(t, hs.URL+"/files/"+it.ID, tok, map[string]string{"Range": "bytes=100-199"})
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != http.StatusPartialContent || !bytes.Equal(b, data[100:200]) {
		t.Fatalf("plage : status=%d len=%d", r.StatusCode, len(b))
	}
	cd := r.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "Mon%20film.mp4") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if r := get(t, hs.URL+"/files/inconnu", tok, nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("id inconnu : %d", r.StatusCode)
	}
}

func TestBookmarkletAndAddRedirect(t *testing.T) {
	code := Bookmarklet("http://192.168.1.5:52780")
	if !strings.HasPrefix(code, "javascript:") || !strings.Contains(code, "http://192.168.1.5:52780") || len(code) > 2048 {
		t.Fatalf("signet invalide (%d octets)", len(code))
	}
	_, _, hs := newTestServer(t)
	resp, err := noRedirect().Get(hs.URL + "/add?u=https%3A%2F%2Fexample.com%2Fv")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/#/remote/add?u=https%3A%2F%2Fexample.com%2Fv" {
		t.Fatalf("redirection = %q", loc)
	}
}
