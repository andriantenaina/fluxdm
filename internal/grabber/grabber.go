// Package grabber is the site spider: it crawls pages from a start URL, finds
// files matching filters (pictures, archives, videos...) and adds them to the
// download list; it can also save the pages for offline browsing and re-run
// periodically to fetch only new files.
package grabber

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
)

// Project is a saved grabber configuration.
type Project struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	StartURL      string     `json:"startUrl"`
	Depth         int        `json:"depth"`         // link levels to follow (0 = start page only)
	SameHost      bool       `json:"sameHost"`      // stay on the start host
	PathPrefix    bool       `json:"pathPrefix"`    // stay under the start path
	MaxPages      int        `json:"maxPages"`      // safety limit
	Include       []string   `json:"include"`       // "jpg", "*.pdf", "re:..." ; empty = all files
	Exclude       []string   `json:"exclude"`       // same syntax
	MinSize       int64      `json:"minSize"`       // skip files smaller than this (bytes, needs HEAD)
	Dir           string     `json:"dir"`           // destination
	SavePages     bool       `json:"savePages"`     // mirror pages for offline browsing
	AutoAdd       bool       `json:"autoAdd"`       // add found files without review
	Queue         string     `json:"queue"`         //
	Cookie        string     `json:"cookie"`        // session cookie for protected sites
	PeriodicHours int        `json:"periodicHours"` // re-run every N hours (0 = never)
	LastRun       *time.Time `json:"lastRun,omitempty"`
	Seen          []string   `json:"seen,omitempty"` // files already added (sync)
}

// Found is a file discovered by the crawler.
type Found struct {
	URL  string `json:"url"`
	Page string `json:"page"`
	Text string `json:"text,omitempty"`
	Type string `json:"type"` // image | video | audio | archive | document | program | other
	Ext  string `json:"ext"`
	New  bool   `json:"new"` // not seen in a previous run
}

// RunStatus is pushed to the UI while crawling.
type RunStatus struct {
	ProjectID string  `json:"projectId"`
	Running   bool    `json:"running"`
	Pages     int     `json:"pages"`
	Queued    int     `json:"queued"`
	Found     []Found `json:"found,omitempty"`
	FoundN    int     `json:"foundCount"`
	Added     int     `json:"added"`
	Error     string  `json:"error,omitempty"`
	Current   string  `json:"current,omitempty"`
}

// Grabber manages projects and runs.
type Grabber struct {
	m        *core.Manager
	OnStatus func(RunStatus)

	mu       sync.Mutex
	projects []Project
	running  map[string]context.CancelFunc
}

// New loads saved projects.
func New(m *core.Manager) *Grabber {
	g := &Grabber{m: m, running: map[string]context.CancelFunc{}}
	_, _ = m.Store.GetSetting("grabber", &g.projects)
	return g
}

// Projects lists projects.
func (g *Grabber) Projects() []Project {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Project, len(g.projects))
	copy(out, g.projects)
	for i := range out {
		out[i].Seen = nil
	}
	return out
}

func (g *Grabber) saveLocked() error { return g.m.Store.SetSetting("grabber", g.projects) }

// Save creates or updates a project.
func (g *Grabber) Save(p Project) (Project, error) {
	if _, err := url.ParseRequestURI(p.StartURL); err != nil {
		return p, errors.New("adresse de départ invalide")
	}
	if p.MaxPages <= 0 {
		p.MaxPages = 500
	}
	if p.Name == "" {
		u, _ := url.Parse(p.StartURL)
		p.Name = u.Host
	}
	if p.Dir == "" {
		p.Dir = filepath.Join(g.m.Settings().DownloadDir, "Grabber", engine.SanitizeFileName(p.Name))
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if p.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		p.ID = hex.EncodeToString(b)
		g.projects = append(g.projects, p)
	} else {
		for i := range g.projects {
			if g.projects[i].ID == p.ID {
				p.Seen = g.projects[i].Seen
				p.LastRun = g.projects[i].LastRun
				g.projects[i] = p
			}
		}
	}
	return p, g.saveLocked()
}

// Delete removes a project.
func (g *Grabber) Delete(id string) error {
	g.Stop(id)
	g.mu.Lock()
	defer g.mu.Unlock()
	out := g.projects[:0]
	for _, p := range g.projects {
		if p.ID != id {
			out = append(out, p)
		}
	}
	g.projects = out
	return g.saveLocked()
}

// Stop cancels a running crawl.
func (g *Grabber) Stop(id string) {
	g.mu.Lock()
	c := g.running[id]
	g.mu.Unlock()
	if c != nil {
		c()
	}
}

func (g *Grabber) project(id string) (Project, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.projects {
		if p.ID == id {
			return p, true
		}
	}
	return Project{}, false
}

// Start runs a project in the background.
func (g *Grabber) Start(id string) error {
	p, ok := g.project(id)
	if !ok {
		return errors.New("projet introuvable")
	}
	g.mu.Lock()
	if _, busy := g.running[id]; busy {
		g.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.running[id] = cancel
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			delete(g.running, id)
			g.mu.Unlock()
		}()
		g.run(ctx, p)
	}()
	return nil
}

// Periodic re-runs projects whose interval elapsed (call from a background loop).
func (g *Grabber) Periodic(stop <-chan struct{}) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			for _, p := range g.Projects() {
				if p.PeriodicHours > 0 && (p.LastRun == nil || time.Since(*p.LastRun) > time.Duration(p.PeriodicHours)*time.Hour) {
					_ = g.Start(p.ID)
				}
			}
		}
	}
}

var typeByExt = map[string]string{}

func init() {
	for t, exts := range map[string]string{
		"image":    "jpg jpeg png gif webp bmp tif tiff svg heic avif ico",
		"video":    "mp4 mkv avi mov webm flv wmv ts m4v 3gp mpg mpeg ogv m3u8",
		"audio":    "mp3 m4a aac flac wav ogg opus wma",
		"archive":  "zip rar 7z gz tgz tar bz2 xz zst iso",
		"document": "pdf doc docx xls xlsx ppt pptx odt ods odp txt rtf epub csv",
		"program":  "exe msi dmg pkg apk deb rpm appimage jar",
	} {
		for _, e := range strings.Fields(exts) {
			typeByExt[e] = t
		}
	}
}

var pageExts = map[string]bool{"": true, "html": true, "htm": true, "php": true, "asp": true, "aspx": true, "jsp": true, "shtml": true, "cgi": true, "xhtml": true}

func extOf(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")
}

func matches(patterns []string, u *url.URL, ext string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
		case strings.HasPrefix(p, "re:"):
			if re, err := regexp.Compile(p[3:]); err == nil && re.MatchString(u.String()) {
				return true
			}
		case strings.ContainsAny(p, "*?"):
			if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(path.Base(u.Path))); ok {
				return true
			}
		case typeByExt[ext] == strings.ToLower(p) || strings.EqualFold(strings.TrimPrefix(p, "."), ext):
			return true
		}
	}
	return false
}

type link struct {
	raw  string // attribute value as written in the page
	abs  *url.URL
	text string
	tag  string
}

// extractLinks returns href/src/srcset targets of a page.
func extractLinks(body []byte, base *url.URL) []link {
	var out []link
	z := html.NewTokenizer(bytes.NewReader(body))
	var lastA *link
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return out
		case html.TextToken:
			if lastA != nil {
				lastA.text += strings.TrimSpace(string(z.Text()))
			}
		case html.EndTagToken:
			if n, _ := z.TagName(); string(n) == "a" && lastA != nil {
				out = append(out, *lastA)
				lastA = nil
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			if tag == "base" {
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "href" {
						if b, err := base.Parse(string(v)); err == nil {
							base = b
						}
					}
				}
				continue
			}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				key, val := string(k), strings.TrimSpace(string(v))
				if val == "" || strings.HasPrefix(val, "#") || strings.HasPrefix(val, "javascript:") || strings.HasPrefix(val, "mailto:") || strings.HasPrefix(val, "data:") {
					continue
				}
				switch {
				case key == "href" && (tag == "a" || tag == "link" || tag == "area"),
					key == "src" && (tag == "img" || tag == "source" || tag == "video" || tag == "audio" || tag == "embed" || tag == "iframe" || tag == "frame" || tag == "script" || tag == "track"),
					key == "data-src" || key == "data-original" || key == "poster":
					if u, err := base.Parse(val); err == nil {
						l := link{raw: val, abs: u, tag: tag}
						if tag == "a" {
							lastA = &l
						} else {
							out = append(out, l)
						}
					}
				case key == "srcset":
					for _, part := range strings.Split(val, ",") {
						f := strings.Fields(strings.TrimSpace(part))
						if len(f) > 0 {
							if u, err := base.Parse(f[0]); err == nil {
								out = append(out, link{raw: f[0], abs: u, tag: tag})
							}
						}
					}
				}
			}
		}
	}
}

func (g *Grabber) fetch(ctx context.Context, cl *http.Client, ua, cookie, target, referer string) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("User-Agent", ua)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", "", errors.New(resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "html") && !strings.Contains(ct, "xml") {
		return nil, ct, resp.Request.URL.String(), nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return b, ct, resp.Request.URL.String(), err
}

// localPath maps a URL into the mirror folder.
func localPath(root string, u *url.URL, isPage bool) string {
	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") {
		p += "index.html"
	} else if isPage && path.Ext(p) == "" {
		p += "/index.html"
	}
	if isPage && u.RawQuery != "" {
		ext := path.Ext(p)
		p = strings.TrimSuffix(p, ext) + "_" + engine.SanitizeFileName(u.RawQuery) + ext
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range parts {
		parts[i] = engine.SanitizeFileName(s)
	}
	return filepath.Join(append([]string{root, engine.SanitizeFileName(u.Host)}, parts...)...)
}

func relLink(fromFile, toFile string) string {
	r, err := filepath.Rel(filepath.Dir(fromFile), toFile)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r)
}

func (g *Grabber) run(ctx context.Context, p Project) {
	st := RunStatus{ProjectID: p.ID, Running: true}
	emit := func() {
		if g.OnStatus != nil {
			s := st
			if len(s.Found) > 2000 {
				s.Found = s.Found[len(s.Found)-2000:]
			}
			g.OnStatus(s)
		}
	}
	start, err := url.Parse(p.StartURL)
	if err != nil {
		st.Running, st.Error = false, err.Error()
		emit()
		return
	}
	cl := g.m.HTTPClient()
	ua := g.m.EngineConfig().UserAgent
	seenBefore := map[string]bool{}
	for _, s := range p.Seen {
		seenBefore[s] = true
	}

	type pageJob struct {
		u     *url.URL
		depth int
		ref   string
	}
	visited := map[string]bool{start.String(): true}
	files := map[string]bool{}
	queue := []pageJob{{u: start}}
	lastEmit := time.Now()
	type savedPage struct {
		file  string
		body  []byte
		links []link
	}
	var saved []savedPage

	inScope := func(u *url.URL) bool {
		if u.Scheme != "http" && u.Scheme != "https" {
			return false
		}
		if p.SameHost && !strings.EqualFold(u.Host, start.Host) {
			return false
		}
		if p.PathPrefix {
			dir := start.Path
			if !strings.HasSuffix(dir, "/") {
				dir = path.Dir(dir) + "/"
			}
			if !strings.HasPrefix(u.Path, dir) {
				return false
			}
		}
		return true
	}

	for len(queue) > 0 && ctx.Err() == nil && st.Pages < p.MaxPages {
		job := queue[0]
		queue = queue[1:]
		st.Current = job.u.String()
		body, ct, final, err := g.fetch(ctx, cl, ua, p.Cookie, job.u.String(), job.ref)
		if err != nil {
			continue
		}
		if body == nil {
			// not HTML: treat as a file
			fu, _ := url.Parse(final)
			if fu != nil && !files[fu.String()] && (len(p.Include) == 0 || matches(p.Include, fu, extOf(fu))) && !matches(p.Exclude, fu, extOf(fu)) {
				files[fu.String()] = true
				st.Found = append(st.Found, Found{URL: fu.String(), Page: job.ref, Type: typeByExt[extOf(fu)], Ext: extOf(fu), New: !seenBefore[fu.String()]})
			}
			_ = ct
			continue
		}
		st.Pages++
		fu, _ := url.Parse(final)
		if fu == nil {
			fu = job.u
		}
		links := extractLinks(body, fu)
		if p.SavePages {
			saved = append(saved, savedPage{file: localPath(p.Dir, fu, true), body: body, links: links})
		}
		for _, l := range links {
			u := l.abs
			u.Fragment = ""
			key := u.String()
			ext := extOf(u)
			isPage := pageExts[ext] && (l.tag == "a" || l.tag == "iframe" || l.tag == "frame" || l.tag == "area")
			if isPage {
				if !visited[key] && job.depth < p.Depth && inScope(u) {
					visited[key] = true
					queue = append(queue, pageJob{u: u, depth: job.depth + 1, ref: fu.String()})
				}
				continue
			}
			if ext == "" || files[key] || ext == "js" || ext == "css" && !p.SavePages {
				continue
			}
			if len(p.Include) > 0 && !matches(p.Include, u, ext) && !(p.SavePages && (ext == "css" || l.tag == "img")) {
				continue
			}
			if matches(p.Exclude, u, ext) {
				continue
			}
			files[key] = true
			t := typeByExt[ext]
			if t == "" {
				t = "other"
			}
			st.Found = append(st.Found, Found{URL: key, Page: fu.String(), Text: l.text, Type: t, Ext: ext, New: !seenBefore[key]})
		}
		st.FoundN = len(st.Found)
		st.Queued = len(queue)
		if time.Since(lastEmit) > 700*time.Millisecond {
			lastEmit = time.Now()
			emit()
		}
	}

	// offline mirror: rewrite links to local paths and save pages
	if p.SavePages {
		local := map[string]string{}
		for _, sp := range saved {
			for _, l := range sp.links {
				k := l.abs.String()
				if visited[k] {
					local[k] = localPath(p.Dir, l.abs, true)
				} else if files[k] {
					local[k] = localPath(p.Dir, l.abs, false)
				}
			}
		}
		for _, sp := range saved {
			out := sp.body
			for _, l := range sp.links {
				if lp, ok := local[l.abs.String()]; ok {
					if rel := relLink(sp.file, lp); rel != "" {
						out = bytes.ReplaceAll(out, []byte(`"`+l.raw+`"`), []byte(`"`+rel+`"`))
						out = bytes.ReplaceAll(out, []byte(`'`+l.raw+`'`), []byte(`'`+rel+`'`))
					}
				}
			}
			_ = os.MkdirAll(filepath.Dir(sp.file), 0o755)
			_ = os.WriteFile(sp.file, out, 0o644)
		}
	}

	// add files
	if p.AutoAdd || p.SavePages {
		for _, f := range st.Found {
			if !f.New {
				continue // already added by a previous run
			}
			req := core.AddRequest{URL: f.URL, PageURL: f.Page, Queue: p.Queue, Start: true,
				Headers: map[string]string{"Referer": f.Page}}
			if p.Cookie != "" {
				req.Headers["Cookie"] = p.Cookie
			}
			if p.SavePages {
				fu, _ := url.Parse(f.URL)
				lp := localPath(p.Dir, fu, false)
				req.Dir, req.FileName = filepath.Dir(lp), filepath.Base(lp)
			} else {
				req.Dir = p.Dir
			}
			if _, err := g.m.Add(req); err == nil {
				st.Added++
			}
		}
	}

	// remember what was found (periodic sync adds only new files)
	g.mu.Lock()
	for i := range g.projects {
		if g.projects[i].ID == p.ID {
			now := time.Now()
			g.projects[i].LastRun = &now
			set := map[string]bool{}
			for _, s := range g.projects[i].Seen {
				set[s] = true
			}
			for _, f := range st.Found {
				set[f.URL] = true
			}
			g.projects[i].Seen = g.projects[i].Seen[:0]
			for s := range set {
				g.projects[i].Seen = append(g.projects[i].Seen, s)
			}
			sort.Strings(g.projects[i].Seen)
		}
	}
	_ = g.saveLocked()
	g.mu.Unlock()

	st.Running = false
	st.Current = ""
	if ctx.Err() != nil {
		st.Error = "arrêté"
	}
	st.FoundN = len(st.Found)
	emit()
}
