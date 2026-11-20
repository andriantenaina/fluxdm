// Package bridge is the local HTTP server the browser extension talks to.
//
// Discovery: the app writes <dataDir>/bridge.json {port, token, exe}. The
// native messaging host (fluxdm-host) reads it and hands port+token to the
// extension (launching the app if needed). As a fallback the extension calls
// GET /v1/hello on the default port: the token is only returned to requests
// whose Origin is a browser extension (web pages cannot forge Origin).
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultPort is tried first so the extension can find the app without native messaging.
const DefaultPort = 52775

// HostName is the native messaging host name.
const HostName = "com.fluxdm.host"

// ExtensionIDs are the extension IDs allowed by the native messaging manifest.
// The first one is derived from extension/keys (fixed "key" in manifest.json);
// add store-assigned IDs here when published.
var ExtensionIDs = []string{
	"lnbmofjmdcfihhamjdgjdhbclmhpfblk",
}

// Discovery is the content of bridge.json.
type Discovery struct {
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Exe     string `json:"exe"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

// DownloadReq is a captured browser download.
type DownloadReq struct {
	URL       string            `json:"url"`
	FileName  string            `json:"fileName,omitempty"`
	Size      int64             `json:"size,omitempty"`
	Mime      string            `json:"mime,omitempty"`
	Referer   string            `json:"referer,omitempty"`
	PageURL   string            `json:"pageUrl,omitempty"`
	Cookies   string            `json:"cookies,omitempty"`
	UserAgent string            `json:"userAgent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Silent    bool              `json:"silent,omitempty"` // add without dialog
}

// Link is one entry of a "download all links" request.
type Link struct {
	URL   string `json:"url"`
	Text  string `json:"text,omitempty"`
	Type  string `json:"type,omitempty"` // link | image | video | audio
	Size  int64  `json:"size,omitempty"`
	Title string `json:"title,omitempty"`
}

// BatchReq is "download all links of the page".
type BatchReq struct {
	PageURL   string `json:"pageUrl"`
	Cookies   string `json:"cookies,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Links     []Link `json:"links"`
}

// MediaVariant is one detected quality for a stream (from the extension).
type MediaVariant struct {
	URL        string `json:"url"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Bandwidth  int64  `json:"bandwidth,omitempty"`
	Label      string `json:"label,omitempty"`
	Mime       string `json:"mime,omitempty"`
	Size       int64  `json:"size,omitempty"`
	StreamKind string `json:"kind,omitempty"` // hls | dash | direct
}

// VideoReq asks the app to download a detected video.
type VideoReq struct {
	Kind      string            `json:"kind"` // hls | dash | direct | page
	URL       string            `json:"url"`  // manifest / media / page URL
	PageURL   string            `json:"pageUrl,omitempty"`
	Title     string            `json:"title,omitempty"`
	Referer   string            `json:"referer,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	Cookies   string            `json:"cookies,omitempty"`
	UserAgent string            `json:"userAgent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Variants  []MediaVariant    `json:"variants,omitempty"`
	Quick     bool              `json:"quick,omitempty"`    // download without dialog
	FileName  string            `json:"fileName,omitempty"` // final name shown in the extension
	Quality   string            `json:"quality,omitempty"`  // Quality.ID chosen in the extension
	Height    int               `json:"height,omitempty"`   // chosen height (0 = best)
	AudioOnly bool              `json:"audioOnly,omitempty"`
}

// InspectReq asks the app what a detected media really is (title, qualities, size, format).
type InspectReq struct {
	Kind      string            `json:"kind"` // hls | dash | direct | page
	URL       string            `json:"url"`
	PageURL   string            `json:"pageUrl,omitempty"`
	Title     string            `json:"title,omitempty"`
	FileName  string            `json:"fileName,omitempty"` // from Content-Disposition (direct files)
	Mime      string            `json:"mime,omitempty"`
	Size      int64             `json:"size,omitempty"`
	Referer   string            `json:"referer,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	Cookies   string            `json:"cookies,omitempty"`
	UserAgent string            `json:"userAgent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// Quality is one selectable quality of a media.
type Quality struct {
	ID           string `json:"id"`    // variant URL / rep ID / "best" / "audio"
	Label        string `json:"label"` // "1080p", "Audio", "Original"
	Height       int    `json:"height,omitempty"`
	Bandwidth    int64  `json:"bandwidth,omitempty"`
	SizeEstimate int64  `json:"sizeEstimate,omitempty"` // bytes, 0 = unknown
	Ext          string `json:"ext"`                    // output extension without dot
}

// InspectResp describes the media for display in the extension.
type InspectResp struct {
	Title          string    `json:"title"`
	Duration       float64   `json:"duration"`
	Qualities      []Quality `json:"qualities"`
	DefaultQuality string    `json:"defaultQuality"`
	OutputExt      string    `json:"outputExt"`
	FileName       string    `json:"fileName"` // suggested final name for the default quality
	Live           bool      `json:"live"`
	Thumbnail      string    `json:"thumbnail,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// ExtSettings are the settings the extension needs.
type ExtSettings struct {
	Version        string   `json:"version"`
	CaptureExts    []string `json:"captureExts"`
	CaptureMinSize int64    `json:"captureMinSize"` // bytes
	SkipSites      []string `json:"skipSites"`
	VideoButton    bool     `json:"videoButton"`
	VideoSkipSites []string `json:"videoSkipSites"` // hosts where the video button is hidden
	CaptureEnabled bool     `json:"captureEnabled"` // app-side capture switch
	Enabled        bool     `json:"enabled"`
	YtdlpSites     []string `json:"ytdlpSites"` // hosts handled by yt-dlp at page level
}

// ExtSettingsPatch is a partial update sent by the extension's options page
// (absent fields are left unchanged).
type ExtSettingsPatch struct {
	CaptureExts    *[]string `json:"captureExts,omitempty"`
	CaptureMinSize *int64    `json:"captureMinSize,omitempty"` // bytes
	SkipSites      *[]string `json:"skipSites,omitempty"`
	VideoButton    *bool     `json:"videoButton,omitempty"`
	VideoSkipSites *[]string `json:"videoSkipSites,omitempty"`
	CaptureEnabled *bool     `json:"captureEnabled,omitempty"`
}

// ActiveItem is one download shown in the extension (popup, side panel).
type ActiveItem struct {
	ID            string  `json:"id"`
	FileName      string  `json:"fileName"`
	Title         string  `json:"title,omitempty"`
	Status        string  `json:"status"`
	Size          int64   `json:"size"`
	Downloaded    int64   `json:"downloaded"`
	Speed         float64 `json:"speed"`
	ETA           float64 `json:"eta"` // seconds; -1 unknown, -2 calculating
	ETAConfidence float64 `json:"etaConfidence"`
	Kind          string  `json:"kind"`
	CompletedAt   string  `json:"completedAt,omitempty"` // RFC 3339
}

// ActiveResp is the answer of GET /v1/active.
type ActiveResp struct {
	Items  []ActiveItem `json:"items"`
	Speed  float64      `json:"speed"`
	Active int          `json:"active"`
}

// ShowReq brings the app to the front (optionally on an item's progress window).
type ShowReq struct {
	ID string `json:"id,omitempty"`
}

// ControlReq pauses, resumes or cancels an item.
type ControlReq struct {
	ID     string `json:"id"`
	Action string `json:"action"` // pause | resume | cancel
}

// Handler implements the app side.
type Handler interface {
	// Download adds a captured file; returns the created item id, or "" when
	// the add dialog was opened instead.
	Download(DownloadReq) (string, error)
	// Batch returns the created ids (none when the batch dialog was opened).
	Batch(BatchReq) ([]string, error)
	Video(VideoReq) (string, error)
	Inspect(InspectReq) InspectResp
	ExtSettings() ExtSettings
	SaveExtSettings(ExtSettingsPatch) (ExtSettings, error)
	Active() ActiveResp
	Show(id string) error
	Control(id, action string) error
}

// Server is the running bridge.
type Server struct {
	ln    net.Listener
	srv   *http.Server
	token string
	file  string
}

// Start listens on 127.0.0.1 (default port, else random) and writes bridge.json.
func Start(dataDir, version string, h Handler) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(DefaultPort))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	s := &Server{ln: ln, token: hex.EncodeToString(b), file: filepath.Join(dataDir, "bridge.json")}
	exe, _ := os.Executable()
	disc := Discovery{Port: ln.Addr().(*net.TCPAddr).Port, Token: s.token, Exe: exe, PID: os.Getpid(), Version: version}
	raw, _ := json.MarshalIndent(disc, "", "  ")
	if err := os.WriteFile(s.file, raw, 0o600); err != nil {
		ln.Close()
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/hello", func(w http.ResponseWriter, r *http.Request) {
		if !isExtensionOrigin(r.Header.Get("Origin")) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, map[string]any{"token": s.token, "version": version})
	})
	mux.HandleFunc("/v1/ping", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.ExtSettings())
	}))
	mux.HandleFunc("/v1/download", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req DownloadReq
		if !decode(w, r, &req) {
			return
		}
		id, err := h.Download(req)
		replyWith(w, err, idField(id))
	}))
	mux.HandleFunc("/v1/batch", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req BatchReq
		if !decode(w, r, &req) {
			return
		}
		ids, err := h.Batch(req)
		extra := map[string]any{}
		if len(ids) > 0 {
			extra["ids"] = ids
		}
		replyWith(w, err, extra)
	}))
	mux.HandleFunc("/v1/video", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req VideoReq
		if !decode(w, r, &req) {
			return
		}
		id, err := h.Video(req)
		replyWith(w, err, idField(id))
	}))
	mux.HandleFunc("/v1/active", s.auth(func(w http.ResponseWriter, r *http.Request) {
		resp := h.Active()
		if resp.Items == nil {
			resp.Items = []ActiveItem{}
		}
		writeJSON(w, resp)
	}))
	mux.HandleFunc("/v1/settings", s.auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, h.ExtSettings())
			return
		}
		var req ExtSettingsPatch
		if !decode(w, r, &req) {
			return
		}
		es, err := h.SaveExtSettings(req)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, es)
	}))
	mux.HandleFunc("/v1/show", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req ShowReq
		if r.Method == http.MethodPost && r.ContentLength != 0 {
			if !decode(w, r, &req) {
				return
			}
		} else if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		reply(w, h.Show(req.ID))
	}))
	mux.HandleFunc("/v1/control", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req ControlReq
		if !decode(w, r, &req) {
			return
		}
		switch req.Action {
		case "pause", "resume", "cancel":
		default:
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"ok": false, "error": "action inconnue : " + req.Action})
			return
		}
		if req.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"ok": false, "error": "id manquant"})
			return
		}
		reply(w, h.Control(req.ID, req.Action))
	}))
	mux.HandleFunc("/v1/inspect", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req InspectReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, h.Inspect(req))
	}))
	s.srv = &http.Server{Handler: cors(mux), ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

// Port returns the listening port.
func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// Close stops the server and removes bridge.json.
func (s *Server) Close() {
	s.srv.Close()
	_ = os.Remove(s.file)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-FluxDM-Token")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func isExtensionOrigin(o string) bool {
	return strings.HasPrefix(o, "chrome-extension://") || strings.HasPrefix(o, "moz-extension://") || strings.HasPrefix(o, "extension://")
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o := r.Header.Get("Origin")
		if o != "" && isExtensionOrigin(o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-FluxDM-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		} else if o != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func reply(w http.ResponseWriter, err error) { replyWith(w, err, nil) }

// replyWith answers {ok:true, ...extra} or {ok:false, error}.
func replyWith(w http.ResponseWriter, err error, extra map[string]any) {
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{"ok": true}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, out)
}

func idField(id string) map[string]any {
	if id == "" {
		return nil
	}
	return map[string]any{"id": id}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ReadDiscovery loads bridge.json.
func ReadDiscovery(dataDir string) (*Discovery, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, "bridge.json"))
	if err != nil {
		return nil, err
	}
	var d Discovery
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	if d.Port == 0 {
		return nil, errors.New("bridge.json invalide")
	}
	return &d, nil
}

// Alive checks that the app answers on the discovered port.
func Alive(d *Discovery) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(d.Port)+"/v1/ping", nil)
	req.Header.Set("X-FluxDM-Token", d.Token)
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// NativeManifest returns the native messaging host manifest JSON.
func NativeManifest(hostExe string) []byte {
	origins := make([]string, 0, len(ExtensionIDs))
	for _, id := range ExtensionIDs {
		origins = append(origins, "chrome-extension://"+id+"/")
	}
	m := map[string]any{
		"name":            HostName,
		"description":     "FluxDM integration",
		"path":            hostExe,
		"type":            "stdio",
		"allowed_origins": origins,
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	return raw
}
