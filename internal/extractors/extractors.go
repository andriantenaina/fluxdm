// Package extractors resolves video host embed pages (Voe, Doodstream, Uqload,
// Vidmoly, Filemoon, StreamWish, Mixdrop, Streamtape...) into direct media URLs.
// Hosts change often, so the logic is data-driven (hosters.json) and can be
// updated without recompiling.
package extractors

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed hosters.json
var builtinRules []byte

// Step is one operation of a hoster recipe.
type Step struct {
	Op       string `json:"op"`
	URL      string `json:"url,omitempty"`
	Referer  string `json:"referer,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Group    int    `json:"group,omitempty"`
	Var      string `json:"var,omitempty"`
	Source   string `json:"source,omitempty"`
	To       string `json:"to,omitempty"`
	With     string `json:"with,omitempty"`
	Value    string `json:"value,omitempty"`
	Fn       string `json:"fn,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Unless   string `json:"unless,omitempty"`   // skip if this var is already set
	When     string `json:"when,omitempty"`     // regex the input var must match
	WhenVar  string `json:"when_var,omitempty"` // skip unless this var is set
}

// Hoster is a recipe for one family of hosts.
type Hoster struct {
	Name    string            `json:"name"`
	Hosts   []string          `json:"hosts"`
	Steps   []Step            `json:"steps"`
	Headers map[string]string `json:"headers,omitempty"`
	re      []*regexp.Regexp
}

// Rules is the content of hosters.json.
type Rules struct {
	Version int       `json:"version"`
	Hosters []*Hoster `json:"hosters"`
}

// Result is a resolved stream.
type Result struct {
	Hoster  string            `json:"hoster"`
	URL     string            `json:"url"`
	Kind    string            `json:"kind"` // hls | dash | http
	Headers map[string]string `json:"headers"`
	Title   string            `json:"title,omitempty"`
}

// Engine holds the active rules.
type Engine struct {
	mu       sync.RWMutex
	rules    *Rules
	dataFile string
	client   func() *http.Client
	ua       func() string
}

// New loads built-in rules, overridden by <dataDir>/rules/hosters.json if newer.
func New(dataDir string, client func() *http.Client, ua func() string) *Engine {
	e := &Engine{dataFile: filepath.Join(dataDir, "rules", "hosters.json"), client: client, ua: ua}
	r, err := parseRules(builtinRules)
	if err != nil {
		panic(err)
	}
	e.rules = r
	if raw, err := os.ReadFile(e.dataFile); err == nil {
		if r2, err := parseRules(raw); err == nil && r2.Version >= r.Version {
			e.rules = r2
		}
	}
	return e
}

func parseRules(raw []byte) (*Rules, error) {
	var r Rules
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	for _, h := range r.Hosters {
		for _, p := range h.Hosts {
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", h.Name, err)
			}
			h.re = append(h.re, re)
		}
		for _, s := range h.Steps {
			if s.Pattern != "" {
				if _, err := regexp.Compile(s.Pattern); err != nil {
					return nil, fmt.Errorf("%s : %w", h.Name, err)
				}
			}
		}
	}
	return &r, nil
}

// Version returns the active rules version.
func (e *Engine) Version() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rules.Version
}

// Names lists supported hosters.
func (e *Engine) Names() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []string
	for _, h := range e.rules.Hosters {
		out = append(out, h.Name)
	}
	return out
}

// Match returns the hoster handling rawURL, or nil.
func (e *Engine) Match(rawURL string) *Hoster {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, h := range e.rules.Hosters {
		for _, re := range h.re {
			if re.MatchString(host) {
				return h
			}
		}
	}
	return nil
}

// Update downloads newer rules from rulesURL.
func (e *Engine) Update(ctx context.Context, rulesURL string) (bool, error) {
	if rulesURL == "" {
		return false, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rulesURL, nil)
	resp, err := e.client().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("règles : %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return false, err
	}
	r, err := parseRules(raw)
	if err != nil {
		return false, err
	}
	if r.Version <= e.Version() {
		return false, nil
	}
	_ = os.MkdirAll(filepath.Dir(e.dataFile), 0o755)
	if err := os.WriteFile(e.dataFile, raw, 0o644); err != nil {
		return false, err
	}
	e.mu.Lock()
	e.rules = r
	e.mu.Unlock()
	return true, nil
}

type runCtx struct {
	ctx     context.Context
	e       *Engine
	vars    map[string]string
	headers map[string]string // from the browser (cookies, UA)
}

var placeholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func (r *runCtx) expand(t string) string {
	return placeholderRe.ReplaceAllStringFunc(t, func(m string) string {
		k := m[1 : len(m)-1]
		switch k {
		case "random10":
			const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
			b := make([]byte, 10)
			for i := range b {
				b[i] = letters[rand.Intn(len(letters))]
			}
			return string(b)
		case "now_ms":
			return strconv.FormatInt(time.Now().UnixMilli(), 10)
		}
		return r.vars[k]
	})
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (r *runCtx) fetch(target, referer string) (string, string, error) {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", err
	}
	ua := r.headers["User-Agent"]
	if ua == "" {
		ua = r.e.ua()
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,en;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if c := r.headers["Cookie"]; c != "" {
		req.Header.Set("Cookie", c)
	}
	resp, err := r.e.client().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("HTTP %s sur %s", resp.Status, target)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), resp.Request.URL.String(), err
}

var jsRedirectRe = regexp.MustCompile(`(?:window\.)?location(?:\.href)?\s*=\s*['"]([^'"]+)['"]|location\.replace\(\s*['"]([^'"]+)['"]`)

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>([^<]+)</title>`)

func (r *runCtx) step(s Step) error {
	if s.Unless != "" && r.vars[s.Unless] != "" {
		return nil
	}
	if s.WhenVar != "" && r.vars[s.WhenVar] == "" {
		return nil
	}
	fail := func(err error) error {
		if s.Optional {
			return nil
		}
		return err
	}
	switch s.Op {
	case "replace":
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return err
		}
		r.vars[s.Var] = re.ReplaceAllString(r.vars[s.Var], s.With)
	case "fetch":
		target := r.expand(s.URL)
		if target == "" {
			return fail(errors.New("URL vide"))
		}
		ref := r.expand(s.Referer)
		if ref == "" {
			ref = r.vars["referer"]
		}
		body, final, err := r.fetch(target, ref)
		if err != nil {
			return fail(err)
		}
		to := s.To
		if to == "" {
			to = "page"
		}
		r.vars[to] = body
		if to == "page" {
			r.vars["referer"] = final
			r.vars["url"] = final
			r.vars["origin"] = originOf(final)
			if m := titleRe.FindStringSubmatch(body); m != nil && r.vars["title"] == "" {
				r.vars["title"] = strings.TrimSpace(m[1])
			}
		}
	case "jsredirect":
		page := r.vars["page"]
		if len(page) > 20000 {
			return nil // a real player page, not a redirect stub
		}
		m := jsRedirectRe.FindStringSubmatch(page)
		if m == nil {
			return nil
		}
		target := m[1]
		if target == "" {
			target = m[2]
		}
		if ref, err := url.Parse(target); err == nil {
			if base, err := url.Parse(r.vars["url"]); err == nil {
				target = base.ResolveReference(ref).String()
			}
		}
		body, final, err := r.fetch(target, r.vars["url"])
		if err != nil {
			return fail(err)
		}
		r.vars["page"], r.vars["url"], r.vars["referer"], r.vars["origin"] = body, final, final, originOf(final)
	case "unpack":
		if u := Unpack(r.vars["page"]); len(u) > 0 {
			r.vars["page"] += "\n" + strings.Join(u, "\n")
		}
	case "regex":
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return err
		}
		src := s.Source
		if src == "" {
			src = "page"
		}
		m := re.FindStringSubmatch(r.vars[src])
		if m == nil || s.Group >= len(m) {
			return fail(fmt.Errorf("motif introuvable (%s)", s.Var))
		}
		r.vars[s.Var] = jsUnescape(m[s.Group])
	case "decode":
		in := r.vars[s.Var]
		if in == "" {
			return fail(fmt.Errorf("rien à décoder (%s)", s.Var))
		}
		if s.When != "" {
			if ok, _ := regexp.MatchString(s.When, in); !ok {
				return nil
			}
		}
		fn := decoders[s.Fn]
		if fn == nil {
			return fmt.Errorf("décodeur inconnu : %s", s.Fn)
		}
		out, err := fn(in)
		if err != nil {
			return fail(err)
		}
		to := s.To
		if to == "" {
			to = s.Var
		}
		r.vars[to] = out
	case "copy":
		if r.vars[s.Var] == "" {
			return fail(fmt.Errorf("variable vide (%s)", s.Var))
		}
		r.vars[s.To] = r.vars[s.Var]
	case "template":
		r.vars[s.To] = r.expand(s.Value)
	case "streamtape":
		out, err := streamtapeURL(r.vars[s.Var])
		if err != nil {
			return fail(err)
		}
		r.vars[s.To] = out
	default:
		return fmt.Errorf("opération inconnue : %s", s.Op)
	}
	return nil
}

var stParts = regexp.MustCompile(`'([^']*)'\s*\+\s*\('([^']*)'\)((?:\.substring\(\d+\))+)`)
var stSub = regexp.MustCompile(`\.substring\((\d+)\)`)

func streamtapeURL(raw string) (string, error) {
	m := stParts.FindStringSubmatch(raw)
	if m == nil {
		return "", errors.New("lien Streamtape introuvable")
	}
	tail := m[2]
	for _, sm := range stSub.FindAllStringSubmatch(m[3], -1) {
		n, _ := strconv.Atoi(sm[1])
		if n <= len(tail) {
			tail = tail[n:]
		}
	}
	return ensureScheme(m[1]+tail) + "&stream=1", nil
}

// KindOf guesses the stream kind from a media URL.
func KindOf(media string) string {
	p := strings.ToLower(media)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	switch {
	case strings.HasSuffix(p, ".m3u8") || strings.Contains(p, "/hls/") || strings.Contains(p, "master.m3u8"):
		return "hls"
	case strings.HasSuffix(p, ".mpd"):
		return "dash"
	}
	return "http"
}

// Resolve runs the matching hoster recipe (or the generic extractor).
func (e *Engine) Resolve(ctx context.Context, rawURL string, headers map[string]string) (*Result, error) {
	h := e.Match(rawURL)
	if h == nil {
		return e.Generic(ctx, rawURL, headers)
	}
	rc := &runCtx{ctx: ctx, e: e, headers: headers, vars: map[string]string{
		"url": rawURL, "origin": originOf(rawURL), "referer": headers["Referer"],
	}}
	for _, s := range h.Steps {
		if err := rc.step(s); err != nil {
			// fall back to the generic extractor before giving up
			if res, gerr := e.Generic(ctx, rawURL, headers); gerr == nil {
				res.Hoster = h.Name + " (générique)"
				return res, nil
			}
			return nil, fmt.Errorf("%s : %w", h.Name, err)
		}
	}
	media := ensureScheme(rc.vars["media"])
	if media == "" || !strings.HasPrefix(media, "http") {
		return nil, fmt.Errorf("%s : vidéo introuvable", h.Name)
	}
	res := &Result{Hoster: h.Name, URL: media, Kind: KindOf(media), Headers: map[string]string{}, Title: cleanTitle(rc.vars["title"])}
	for k, v := range h.Headers {
		res.Headers[k] = rc.expand(v)
	}
	if ua := headers["User-Agent"]; ua != "" {
		res.Headers["User-Agent"] = ua
	}
	return res, nil
}

var genericRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)file\s*:\s*["']([^"']+\.m3u8[^"']*)["']`),
	regexp.MustCompile(`(?i)["'](https?:[^"'\s]+\.m3u8[^"'\s]*)["']`),
	regexp.MustCompile(`(?i)["'](https?:[^"'\s]+\.mpd[^"'\s]*)["']`),
	regexp.MustCompile(`(?i)file\s*:\s*["']([^"']+\.mp4[^"']*)["']`),
	regexp.MustCompile(`(?i)<source[^>]+src=["']([^"']+)["']`),
	regexp.MustCompile(`(?i)["'](https?:[^"'\s]+\.(?:mp4|webm|mkv)(?:\?[^"'\s]*)?)["']`),
}

// Generic fetches a page and looks for a stream URL (also inside packed scripts).
func (e *Engine) Generic(ctx context.Context, rawURL string, headers map[string]string) (*Result, error) {
	rc := &runCtx{ctx: ctx, e: e, headers: headers, vars: map[string]string{"url": rawURL, "origin": originOf(rawURL)}}
	body, final, err := rc.fetch(rawURL, headers["Referer"])
	if err != nil {
		return nil, err
	}
	all := body + "\n" + strings.Join(Unpack(body), "\n")
	all = jsUnescape(all)
	for _, re := range genericRes {
		if m := re.FindStringSubmatch(all); m != nil {
			media := ensureScheme(m[1])
			if ref, err := url.Parse(media); err == nil {
				if base, err := url.Parse(final); err == nil {
					media = base.ResolveReference(ref).String()
				}
			}
			title := ""
			if t := titleRe.FindStringSubmatch(body); t != nil {
				title = cleanTitle(t[1])
			}
			return &Result{Hoster: "générique", URL: media, Kind: KindOf(media), Title: title,
				Headers: map[string]string{"Referer": final, "Origin": originOf(final)}}, nil
		}
	}
	return nil, errors.New("aucune vidéo trouvée sur la page")
}

func cleanTitle(t string) string {
	t = strings.TrimSpace(t)
	for _, suf := range []string{" - Voe", " - DoodStream", " - Vidmoly", " - Uqload", " - Streamtape", " - MixDrop"} {
		t = strings.TrimSuffix(t, suf)
	}
	for _, pre := range []string{"Watch ", "Regarder "} {
		t = strings.TrimPrefix(t, pre)
	}
	if strings.EqualFold(t, "video") || strings.EqualFold(t, "embed") {
		return ""
	}
	return t
}
