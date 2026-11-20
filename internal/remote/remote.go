// Package remote is "FluxDM Remote": a LAN web server that lets an iPhone (or
// any phone/tablet) control FluxDM from Safari — add links and videos, follow
// progress, fetch finished files — with no app to install and nothing that
// expires. Devices are paired with a one-time code (QR) and authenticated by a
// per-device token (only its SHA-256 is stored).
package remote

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"fluxdm/internal/core"
	"fluxdm/internal/media"
	"fluxdm/internal/store"
)

// Server is the FluxDM Remote HTTP server.
type Server struct {
	m       *core.Manager
	rt      *media.Runtime
	assets  fs.FS
	icon    []byte
	version string

	mu       sync.Mutex
	srv      *http.Server
	ln       net.Listener
	codes    map[string]time.Time // pairing code -> expiry
	attempts map[string][]time.Time
}

// New creates the server (not started).
func New(m *core.Manager, rt *media.Runtime, assets fs.FS, icon []byte, version string) *Server {
	return &Server{m: m, rt: rt, assets: assets, icon: icon, version: version, codes: map[string]time.Time{}, attempts: map[string][]time.Time{}}
}

// Running reports whether the server listens.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil
}

// Port returns the listening port (0 if stopped).
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return 0
	}
	return s.ln.Addr().(*net.TCPAddr).Port
}

// Apply starts or stops the server according to the settings.
func (s *Server) Apply() error {
	rs := s.m.Settings().Remote
	if !rs.Enabled {
		s.Stop()
		return nil
	}
	if s.Running() && s.Port() == rs.Port {
		return nil
	}
	s.Stop()
	return s.Start(rs.Port)
}

// Start listens on all interfaces.
func (s *Server) Start(port int) error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("port %d indisponible : %w", port, err)
	}
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	s.srv, s.ln = srv, ln
	s.mu.Unlock()
	go srv.Serve(ln)
	return nil
}

// Stop closes the server.
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv, s.ln = nil, nil
	s.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// Address is one URL at which the server can be reached.
type Address struct {
	Label string `json:"label"` // Wi-Fi / Ethernet / Tailscale
	URL   string `json:"url"`
	QR    string `json:"qr,omitempty"` // data:image/png;base64,...
}

// Addresses lists the LAN / Tailscale URLs of this computer.
func (s *Server) Addresses() []Address {
	port := s.m.Settings().Remote.Port
	var out []Address
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || virtualIface(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			label := ""
			switch {
			case ip[0] == 100 && ip[1]&0xC0 == 64:
				label = "Tailscale (hors de la maison)"
			case ip.IsPrivate():
				label = "Réseau local (" + ifc.Name + ")"
			default:
				continue
			}
			out = append(out, Address{Label: label, URL: fmt.Sprintf("http://%s:%d", ip, port)})
		}
	}
	return out
}

// virtualIface reports adapters a phone can never reach (Hyper-V/WSL, VMs, Docker).
func virtualIface(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"vethernet", "virtualbox", "vmware", "vmnet", "docker", "wsl", "hyper-v", "vboxnet", "utun", "bridge100"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// NewPairing creates a one-time code valid 10 minutes and the QR codes to scan.
func (s *Server) NewPairing() (string, []Address) {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	code := strings.ToUpper(hex.EncodeToString(b))[:8]
	s.mu.Lock()
	for c, exp := range s.codes {
		if time.Now().After(exp) {
			delete(s.codes, c)
		}
	}
	s.codes[code] = time.Now().Add(10 * time.Minute)
	s.mu.Unlock()
	addrs := s.Addresses()
	for i := range addrs {
		u := addrs[i].URL + "/pair?code=" + code
		addrs[i].URL = u
		if png, err := qrcode.Encode(u, qrcode.Medium, 300); err == nil {
			addrs[i].QR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}
	return code, addrs
}

// Revoke removes a paired device.
func (s *Server) Revoke(id string) error {
	set := s.m.Settings()
	out := set.Remote.Devices[:0]
	for _, d := range set.Remote.Devices {
		if d.ID != id {
			out = append(out, d)
		}
	}
	set.Remote.Devices = out
	return s.m.SetSettings(set)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func clientIP(r *http.Request) string {
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

// allowAttempt limits pairing attempts to 5 per minute per IP.
func (s *Server) allowAttempt(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range s.attempts[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 5 {
		s.attempts[ip] = keep
		return false
	}
	s.attempts[ip] = append(keep, now)
	return true
}

func deviceName(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "Macintosh"):
		return "Mac"
	}
	return "Appareil"
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	if !s.allowAttempt(clientIP(r)) {
		http.Error(w, "trop de tentatives, réessayez dans une minute", http.StatusTooManyRequests)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	s.mu.Lock()
	exp, ok := s.codes[code]
	if ok {
		delete(s.codes, code) // one-time
	}
	s.mu.Unlock()
	if !ok || time.Now().After(exp) {
		http.Error(w, "code d'appairage invalide ou expiré : générez-en un nouveau dans FluxDM", http.StatusUnauthorized)
		return
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	idb := make([]byte, 6)
	_, _ = rand.Read(idb)
	set := s.m.Settings()
	set.Remote.Devices = append(set.Remote.Devices, core.RemoteDevice{
		ID: hex.EncodeToString(idb), Name: deviceName(r.UserAgent()), TokenHash: hashToken(token),
		CreatedAt: time.Now().Unix(), LastSeen: time.Now().Unix(),
	})
	if err := s.m.SetSettings(set); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setTokenCookie(w, token)
	// The token stays in the page URL: on iOS a web app added to the home
	// screen does not share Safari's cookies, it starts from this URL instead.
	http.Redirect(w, r, "/?k="+token+"#/remote", http.StatusFound)
}

func setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: "fdm_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 10 * 365 * 24 * 3600})
}

// validToken reports whether t belongs to a paired device.
func (s *Server) validToken(t string) bool {
	if len(t) < 32 {
		return false
	}
	h := hashToken(t)
	for _, d := range s.m.Settings().Remote.Devices {
		if d.TokenHash == h {
			return true
		}
	}
	return false
}

// tokensOf returns the candidate tokens of a request (header, cookie, query):
// a stale cookie must not hide a valid header token.
func tokensOf(r *http.Request) []string {
	var out []string
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		out = append(out, strings.TrimSpace(strings.TrimPrefix(a, "Bearer ")))
	}
	if c, err := r.Cookie("fdm_token"); err == nil && c.Value != "" {
		out = append(out, c.Value)
	}
	q := r.URL.Query()
	for _, k := range []string{"token", "k"} {
		if t := q.Get(k); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// device returns the paired device of the request, or nil.
func (s *Server) device(r *http.Request) *core.RemoteDevice {
	hashes := map[string]bool{}
	for _, t := range tokensOf(r) {
		if len(t) >= 32 {
			hashes[hashToken(t)] = true
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	set := s.m.Settings()
	for i := range set.Remote.Devices {
		d := &set.Remote.Devices[i]
		if hashes[d.TokenHash] {
			if time.Now().Unix()-d.LastSeen > 300 { // refresh "last seen" at most every 5 min
				d.LastSeen = time.Now().Unix()
				_ = s.m.SetSettings(set)
			}
			return d
		}
	}
	return nil
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.device(r) == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// LiteItem is what the phone needs to display.
type LiteItem struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Size       int64      `json:"size"`
	Downloaded int64      `json:"downloaded"`
	Category   string     `json:"category"`
	Error      string     `json:"error,omitempty"`
	Host       string     `json:"host"`
	CreatedAt  time.Time  `json:"createdAt"`
	DoneAt     *time.Time `json:"completedAt,omitempty"`
	Ext        string     `json:"ext"`
}

func lite(it *store.Item) LiteItem {
	name := it.FileName
	if name == "" {
		name = it.Title
	}
	if name == "" {
		name = it.URL
	}
	host := ""
	src := it.PageURL
	if src == "" {
		src = it.URL
	}
	if u, err := url.Parse(src); err == nil {
		host = u.Hostname()
	}
	return LiteItem{ID: it.ID, Kind: it.Kind, Name: name, Status: it.Status, Size: it.Size, Downloaded: it.Downloaded,
		Category: it.Category, Error: it.Error, Host: host, CreatedAt: it.CreatedAt, DoneAt: it.CompletedAt,
		Ext: strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")}
}

// AddReq is sent by the phone (web app, bookmarklet page, Shortcut).
type AddReq struct {
	URL       string `json:"url"`
	PageURL   string `json:"pageUrl,omitempty"`
	Title     string `json:"title,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Quality   string `json:"quality,omitempty"` // Quality.ID from /api/analyze
	Height    int    `json:"height,omitempty"`
	AudioOnly bool   `json:"audioOnly,omitempty"`
	Output    string `json:"output,omitempty"`
	FileName  string `json:"fileName,omitempty"`
	Referer   string `json:"referer,omitempty"`
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var req AddReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
		fail(w, http.StatusBadRequest, errors.New("adresse manquante"))
		return
	}
	h := map[string]string{}
	ref := req.Referer
	if ref == "" && req.PageURL != "" && req.PageURL != req.URL {
		ref = req.PageURL
	}
	if ref != "" {
		h["Referer"] = ref
	}
	opts := map[string]any{}
	if req.Output != "" {
		opts["output"] = req.Output
	}
	switch {
	case req.AudioOnly || req.Quality == "audio":
		opts["audioOnly"] = true
		if req.Output == "" {
			opts["output"] = "mp3"
		}
	case strings.HasPrefix(req.Quality, "rep:"):
		opts["videoRep"] = strings.TrimPrefix(req.Quality, "rep:")
	case strings.HasPrefix(req.Quality, "h:"):
		if n, err := strconv.Atoi(strings.TrimPrefix(req.Quality, "h:")); err == nil {
			opts["maxHeight"] = n
		}
	case strings.HasPrefix(req.Quality, "http"):
		opts["variantUrl"] = req.Quality
	}
	if req.Height > 0 {
		opts["maxHeight"] = req.Height
	}
	kind := req.Kind
	switch kind {
	case "hls", "dash", "ytdlp", "hoster", "http", "torrent", "ftp":
	default:
		kind = ""
	}
	it, err := s.m.Add(core.AddRequest{URL: strings.TrimSpace(req.URL), PageURL: req.PageURL, Title: req.Title, Kind: kind,
		FileName: req.FileName, Headers: h, Options: opts, Start: true})
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, lite(it))
}

func (s *Server) analyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string `json:"url"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Referer string `json:"referer"`
	}
	if err := readJSON(r, &req); err != nil || req.URL == "" {
		fail(w, http.StatusBadRequest, errors.New("adresse manquante"))
		return
	}
	h := map[string]string{}
	if req.Referer != "" {
		h["Referer"] = req.Referer
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	kind := req.Kind
	if kind == "" {
		kind = s.m.DetectKind(req.URL)
	}
	info, err := s.rt.Analyze(ctx, kind, req.URL, h, false)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, map[string]any{"kind": kind, "inspection": s.rt.FromInfo(info, req.Title, kind), "source": info.Source, "playlist": info.Playlist})
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs        []string `json:"ids"`
		DeleteFile bool     `json:"deleteFile"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	act := strings.TrimPrefix(r.URL.Path, "/api/")
	for _, id := range req.IDs {
		switch act {
		case "pause":
			go s.m.Pause(id)
		case "resume":
			_ = s.m.Start(id)
		case "remove":
			_ = s.m.Remove(id, req.DeleteFile)
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) items(w http.ResponseWriter, r *http.Request) {
	all := s.m.List()
	out := make([]LiteItem, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		out = append(out, lite(all[i]))
	}
	writeJSON(w, out)
}

// events streams progress with Server-Sent Events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming non pris en charge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	var mu sync.Mutex
	latest := map[string]*core.Progress{}
	changed := make(chan struct{}, 1)
	unsub := s.m.Subscribe(func(ev core.Event) {
		mu.Lock()
		switch ev.Type {
		case "progress":
			latest[ev.ID] = ev.Progress
		default:
			latest["_items"] = nil
		}
		mu.Unlock()
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer unsub()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-t.C:
			mu.Lock()
			if len(latest) == 0 {
				mu.Unlock()
				continue
			}
			_, itemsChanged := latest["_items"]
			delete(latest, "_items")
			var batch []*core.Progress
			for _, p := range latest {
				p2 := *p
				p2.Segments = nil
				batch = append(batch, &p2)
			}
			latest = map[string]*core.Progress{}
			mu.Unlock()
			if len(batch) > 0 {
				b, _ := json.Marshal(batch)
				fmt.Fprintf(w, "event: progress\ndata: %s\n\n", b)
			}
			if itemsChanged {
				fmt.Fprint(w, "event: items\ndata: {}\n\n")
			}
			fl.Flush()
		case <-changed:
		}
	}
}

// file sends a finished download (Range supported, proper file name).
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/files/")
	it, err := s.m.Get(id)
	if err != nil || it.Status != store.StatusCompleted || it.Path == "" {
		http.Error(w, "fichier indisponible", http.StatusNotFound)
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		http.Error(w, "fichier introuvable sur le disque", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "fichier invalide", http.StatusNotFound)
		return
	}
	name := filepath.Base(it.Path)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	d := s.device(r)
	writeJSON(w, map[string]any{"version": s.version, "device": d.Name, "base": "http://" + r.Host})
}

// Bookmarklet returns the javascript: code of the Safari bookmark.
func Bookmarklet(base string) string {
	js := `(()=>{const B='` + base + `';const s=new Set();const a=u=>{if(u&&/^https?:/.test(u))s.add(u)};` +
		`document.querySelectorAll('video').forEach(v=>{a(v.currentSrc||v.src);v.querySelectorAll('source').forEach(x=>a(x.src))});` +
		`(performance.getEntriesByType('resource')||[]).forEach(e=>{if(/\.(m3u8|mpd|mp4|webm|m4a|mov)(\?|$)/i.test(e.name))a(e.name)});` +
		`const f=[...document.querySelectorAll('iframe')].map(x=>x.src).filter(u=>/^https?:/.test(u));` +
		`const o=document.querySelector('meta[property="og:title"]');const d={p:location.href,t:(o&&o.content)||document.title,m:[...s].slice(-12),f:f.slice(0,8)};` +
		`let j=JSON.stringify(d);if(j.length>6000){d.m=[];j=JSON.stringify(d)}` +
		`const u=B+'/add?d='+encodeURIComponent(btoa(unescape(encodeURIComponent(j))));if(!window.open(u,'_blank'))location.href=u})()`
	return "javascript:" + js
}

func (s *Server) bookmarklet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"code": Bookmarklet("http://" + r.Host)})
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	start := "/#/remote"
	if k := r.URL.Query().Get("k"); s.validToken(k) {
		start = "/?k=" + k + "#/remote" // per-device start URL (home-screen app has its own storage)
	}
	b, _ := json.Marshal(map[string]any{
		"name": "FluxDM Remote", "short_name": "FluxDM", "start_url": start, "scope": "/", "display": "standalone",
		"background_color": "#14161c", "theme_color": "#2b7fff",
		"icons": []map[string]string{{"src": "/icon.png", "sizes": "256x256", "type": "image/png"}},
	})
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/pair", s.pair)
	mux.HandleFunc("/manifest.webmanifest", s.manifest)
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(s.icon)
	})
	mux.HandleFunc("/apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(s.icon)
	})
	mux.HandleFunc("/api/info", s.auth(s.info))
	mux.HandleFunc("/api/items", s.auth(s.items))
	mux.HandleFunc("/api/events", s.auth(s.events))
	mux.HandleFunc("/api/add", s.auth(s.add))
	mux.HandleFunc("/api/analyze", s.auth(s.analyze))
	mux.HandleFunc("/api/pause", s.auth(s.action))
	mux.HandleFunc("/api/resume", s.auth(s.action))
	mux.HandleFunc("/api/remove", s.auth(s.action))
	mux.HandleFunc("/api/bookmarklet", s.auth(s.bookmarklet))
	mux.HandleFunc("/files/", s.auth(s.file))
	// bookmarklet / Shortcut landing page: hand the query to the web app
	mux.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/#/remote/add?"+r.URL.RawQuery, http.StatusFound)
	})
	static := http.FileServer(http.FS(s.assets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			// inject the PWA tags into the SPA page
			b, err := fs.ReadFile(s.assets, "index.html")
			if err != nil {
				http.Error(w, "UI absente", http.StatusInternalServerError)
				return
			}
			mf := "/manifest.webmanifest"
			if k := r.URL.Query().Get("k"); s.validToken(k) {
				setTokenCookie(w, k)
				mf += "?k=" + url.QueryEscape(k)
			}
			html := strings.Replace(string(b), "</head>", `<link rel="manifest" href="`+mf+`"><link rel="apple-touch-icon" href="/apple-touch-icon.png">`+
				`<meta name="apple-mobile-web-app-capable" content="yes"><meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">`+
				`<meta name="apple-mobile-web-app-title" content="FluxDM"><meta name="theme-color" content="#2b7fff">`+
				`<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"></head>`, 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(html))
			return
		}
		static.ServeHTTP(w, r)
	})
	return securityHeaders(mux)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}
