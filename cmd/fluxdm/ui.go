package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"fluxdm/internal/core"
	"fluxdm/internal/store"
)

// UI owns windows, tray, and pushes manager events to the frontend.
type UI struct {
	app     *application.App
	mgr     *core.Manager
	api     *API
	svc     *services
	version string

	mainWin *application.WebviewWindow
	tray    *application.SystemTray

	mu       sync.Mutex
	pending  map[string]*PendingAdd
	progress map[string]*application.WebviewWindow
	seq      int
	lastClip string

	stopCh     chan struct{}
	background []func(stop <-chan struct{})
}

// PendingAdd is shown by an "add download" dialog window.
type PendingAdd struct {
	Token   string            `json:"token"`
	Request core.AddRequest   `json:"request"`
	Batch   []core.AddRequest `json:"batch,omitempty"` // "download all links"
	Video   bool              `json:"video"`           // open the video quality picker
}

func (u *UI) shutdown() {
	if u.stopCh != nil {
		close(u.stopCh)
	}
}

func (u *UI) createMainWindow() {
	u.pending = map[string]*PendingAdd{}
	u.progress = map[string]*application.WebviewWindow{}
	s := u.mgr.Settings()
	u.mainWin = u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "FluxDM",
		Width:            1180,
		Height:           700,
		MinWidth:         760,
		MinHeight:        420,
		URL:              "/#/main",
		Hidden:           s.StartMinimized || hasArg("--background"),
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(30, 32, 38),
	})
	u.mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if u.mgr.Settings().CloseToTray {
			u.mainWin.Hide()
			e.Cancel()
			return
		}
		u.app.Quit()
	})
	u.mainWin.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		for _, f := range e.Context().DroppedFiles() {
			u.handleArgs([]string{f}, true)
		}
	})
}

// ShowMain brings the main window to the front.
func (u *UI) ShowMain() {
	if u.mainWin == nil {
		return
	}
	u.mainWin.Show()
	u.mainWin.UnMinimise()
	u.mainWin.Focus()
}

func (u *UI) createTray() {
	u.tray = u.app.SystemTray.New()
	u.tray.SetIcon(appIcon)
	u.tray.SetTooltip("FluxDM")
	menu := u.app.NewMenu()
	menu.Add("Ouvrir FluxDM").OnClick(func(*application.Context) { u.ShowMain() })
	menu.Add("Ajouter une URL…").OnClick(func(*application.Context) {
		u.OpenAdd(core.AddRequest{Start: true}, nil, false)
	})
	menu.AddSeparator()
	menu.Add("Tout reprendre").OnClick(func(*application.Context) { u.mgr.StartAll() })
	menu.Add("Tout suspendre").OnClick(func(*application.Context) { u.mgr.PauseAll() })
	menu.AddCheckbox("Surveiller le presse-papier", u.mgr.Settings().ClipboardMonitor).OnClick(func(ctx *application.Context) {
		s := u.mgr.Settings()
		s.ClipboardMonitor = ctx.ClickedMenuItem().Checked()
		_ = u.mgr.SetSettings(s)
	})
	menu.AddSeparator()
	menu.Add("Quitter").OnClick(func(*application.Context) { u.app.Quit() })
	u.tray.SetMenu(menu)
	u.tray.OnClick(func() { u.ShowMain() })
}

func (u *UI) newToken() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.seq++
	return fmt.Sprintf("w%d-%d", time.Now().UnixNano()%1e9, u.seq)
}

// OpenAdd opens an "add download" dialog (or the video picker).
func (u *UI) OpenAdd(req core.AddRequest, batch []core.AddRequest, video bool) {
	tok := u.newToken()
	u.mu.Lock()
	u.pending[tok] = &PendingAdd{Token: tok, Request: req, Batch: batch, Video: video}
	u.mu.Unlock()
	route := "add"
	w, h := 620, 400
	if len(batch) > 0 {
		route, w, h = "batch", 760, 560
	} else if video {
		route, w, h = "video", 640, 520
	}
	win := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "dlg-" + tok,
		Title:            "FluxDM",
		Width:            w,
		Height:           h,
		MinWidth:         480,
		MinHeight:        320,
		URL:              "/#/" + route + "/" + tok,
		AlwaysOnTop:      true,
		BackgroundColour: application.NewRGB(30, 32, 38),
	})
	win.Show()
	win.Focus()
}

// TakePending returns (and keeps until consumed) the request for a dialog token.
func (u *UI) TakePending(tok string) (*PendingAdd, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	p, ok := u.pending[tok]
	if !ok {
		return nil, fmt.Errorf("demande expirée")
	}
	return p, nil
}

// DropPending forgets a dialog token.
func (u *UI) DropPending(tok string) {
	u.mu.Lock()
	delete(u.pending, tok)
	u.mu.Unlock()
}

// OpenProgress opens (or focuses) the progress window of an item.
func (u *UI) OpenProgress(id string) {
	u.mu.Lock()
	if w, ok := u.progress[id]; ok {
		u.mu.Unlock()
		w.Show()
		w.Focus()
		return
	}
	u.mu.Unlock()
	win := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "prog-" + id,
		Title:            "FluxDM — progression",
		Width:            560,
		Height:           420,
		MinWidth:         460,
		MinHeight:        340,
		URL:              "/#/progress/" + id,
		BackgroundColour: application.NewRGB(30, 32, 38),
	})
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		u.mu.Lock()
		delete(u.progress, id)
		u.mu.Unlock()
	})
	u.mu.Lock()
	u.progress[id] = win
	u.mu.Unlock()
	win.Show()
}

func (u *UI) closeProgress(id string) {
	u.mu.Lock()
	w, ok := u.progress[id]
	delete(u.progress, id)
	u.mu.Unlock()
	if ok {
		w.Close()
	}
}

func (u *UI) openDone(id string) {
	win := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "done-" + id,
		Title:            "FluxDM — téléchargement terminé",
		Width:            520,
		Height:           260,
		DisableResize:    true,
		AlwaysOnTop:      true,
		URL:              "/#/done/" + id,
		BackgroundColour: application.NewRGB(30, 32, 38),
	})
	win.Show()
}

// startEventForwarding pushes manager events to all windows; progress is
// coalesced and sent 4 times per second.
func (u *UI) startEventForwarding() {
	var pmu sync.Mutex
	latest := map[string]*core.Progress{}
	u.mgr.Subscribe(func(ev core.Event) {
		switch ev.Type {
		case "progress":
			pmu.Lock()
			latest[ev.ID] = ev.Progress
			pmu.Unlock()
			return
		case "completed":
			u.closeProgress(ev.Item.ID)
			s := u.mgr.Settings()
			if s.ShowDoneDialog && ev.Item.Queue == core.MainQueue {
				u.openDone(ev.Item.ID)
			}
		case "removed":
			u.closeProgress(ev.ID)
		}
		u.app.Event.Emit("fdm:event", ev)
	})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			pmu.Lock()
			if len(latest) == 0 {
				pmu.Unlock()
				continue
			}
			batch := make([]*core.Progress, 0, len(latest))
			for _, p := range latest {
				batch = append(batch, p)
			}
			latest = map[string]*core.Progress{}
			pmu.Unlock()
			u.app.Event.Emit("fdm:progress", batch)
			u.updateTrayTooltip(batch)
		}
	}()
}

func (u *UI) updateTrayTooltip(batch []*core.Progress) {
	var speed float64
	for _, p := range batch {
		speed += p.Speed
	}
	if u.tray != nil {
		u.tray.SetTooltip(fmt.Sprintf("FluxDM — %d actif(s) — %s/s", u.mgr.ActiveCount(), humanSize(int64(speed))))
	}
}

func humanSize(n int64) string {
	units := []string{"o", "Ko", "Mo", "Go", "To"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

// startClipboardMonitor offers to download URLs copied to the clipboard.
func (u *UI) startClipboardMonitor() {
	go func() {
		t := time.NewTicker(1200 * time.Millisecond)
		defer t.Stop()
		first := true
		for range t.C {
			s := u.mgr.Settings()
			if !s.ClipboardMonitor {
				first = true
				continue
			}
			txt, ok := u.app.Clipboard.Text()
			if !ok {
				continue
			}
			txt = strings.TrimSpace(txt)
			if first { // ignore what was already in the clipboard when enabled
				u.lastClip, first = txt, false
				continue
			}
			if txt == u.lastClip {
				continue
			}
			u.lastClip = txt
			if looksDownloadable(txt, s.CaptureExts) {
				u.OpenAdd(core.AddRequest{URL: txt, Start: true}, nil, false)
			}
		}
	}()
}

func looksDownloadable(txt string, exts []string) bool {
	if strings.ContainsAny(txt, " \n\t") {
		return false
	}
	low := strings.ToLower(txt)
	if strings.HasPrefix(low, "magnet:?") {
		return true
	}
	pu, err := url.Parse(txt)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https" && pu.Scheme != "ftp") {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(pu.Path)), ".")
	if ext == "m3u8" || ext == "mpd" {
		return true
	}
	for _, e := range exts {
		if e == ext {
			return true
		}
	}
	return false
}

// AddBackground registers a background task (scheduler, tool updates...).
func (u *UI) AddBackground(fn func(stop <-chan struct{})) {
	u.background = append(u.background, fn)
}

func (u *UI) startBackground() {
	u.stopCh = make(chan struct{})
	for _, fn := range u.background {
		go fn(u.stopCh)
	}
}

// handleArgs processes command-line / second-instance / protocol arguments.
func (u *UI) handleArgs(args []string, focusIfEmpty bool) {
	handled := false
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "" || strings.HasPrefix(a, "-psn_") || a == "--background" {
			continue
		}
		switch {
		case a == "add":
			continue
		case strings.HasPrefix(strings.ToLower(a), "fluxdm://"):
			u.handleProtocol(a)
			handled = true
		case strings.HasPrefix(strings.ToLower(a), "magnet:"),
			strings.HasPrefix(a, "http://"), strings.HasPrefix(a, "https://"), strings.HasPrefix(a, "ftp://"):
			u.OpenAdd(core.AddRequest{URL: a, Start: true}, nil, false)
			handled = true
		default:
			if fi, err := os.Stat(a); err == nil && !fi.IsDir() {
				if strings.HasSuffix(strings.ToLower(a), ".torrent") {
					u.OpenAdd(core.AddRequest{URL: a, Kind: store.KindTorrent, Start: true}, nil, false)
					handled = true
				} else if strings.HasSuffix(strings.ToLower(a), ".txt") {
					u.api.importFile(a)
					handled = true
				}
			}
		}
	}
	if !handled && focusIfEmpty {
		u.ShowMain()
	}
}

// handleProtocol handles fluxdm://add?url=...&referer=...&cookie=...&name=...
func (u *UI) handleProtocol(raw string) {
	pu, err := url.Parse(raw)
	if err != nil {
		return
	}
	q := pu.Query()
	target := q.Get("url")
	if target == "" {
		u.ShowMain()
		return
	}
	h := map[string]string{}
	if v := q.Get("referer"); v != "" {
		h["Referer"] = v
	}
	if v := q.Get("cookie"); v != "" {
		h["Cookie"] = v
	}
	if v := q.Get("ua"); v != "" {
		h["User-Agent"] = v
	}
	u.OpenAdd(core.AddRequest{URL: target, FileName: q.Get("name"), Headers: h, PageURL: q.Get("page"), Start: true}, nil, q.Get("video") == "1")
}

func hasArg(a string) bool {
	for _, x := range os.Args[1:] {
		if x == a {
			return true
		}
	}
	return false
}

// emitJSON is a helper for debugging.
func emitJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
