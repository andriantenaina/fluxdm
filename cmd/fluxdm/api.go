package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

// API is the service bound to the frontend (methods callable as "main.API.<Name>").
type API struct {
	mgr *core.Manager
	ui  *UI
}

// Version returns the application version.
func (a *API) Version() string { return a.ui.version }

// Platform returns "windows" or "darwin".
func (a *API) Platform() string { return runtime.GOOS }

// List returns all downloads.
func (a *API) List() []*store.Item { return a.mgr.List() }

// Get returns one download.
func (a *API) Get(id string) (*store.Item, error) { return a.mgr.Get(id) }

// Settings returns the user settings.
func (a *API) Settings() core.Settings { return a.mgr.Settings() }

// SaveSettings stores the user settings (and applies "launch at startup").
func (a *API) SaveSettings(s core.Settings) error {
	old := a.mgr.Settings()
	if err := a.mgr.SetSettings(s); err != nil {
		return err
	}
	if old.LaunchAtStartup != s.LaunchAtStartup && a.ui.app != nil {
		var err error
		if s.LaunchAtStartup {
			err = a.ui.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "FluxDM", Arguments: []string{"--background"}})
		} else {
			err = a.ui.app.Autostart.Disable()
		}
		if err != nil {
			return fmt.Errorf("démarrage automatique : %w", err)
		}
	}
	return nil
}

// DefaultSettings returns factory settings (for "reset").
func (a *API) DefaultSettings() core.Settings { return core.DefaultSettings() }

// ProbeResult is shown in the add dialog.
type ProbeResult struct {
	Kind        string `json:"kind"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	Resumable   bool   `json:"resumable"`
	ContentType string `json:"contentType"`
	Category    string `json:"category"`
	Dir         string `json:"dir"`
	FinalURL    string `json:"finalUrl"`
}

// Probe inspects a URL before adding it.
func (a *API) Probe(rawURL string, headers map[string]string) (*ProbeResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	kind := a.mgr.DetectKind(rawURL)
	if fi, err := os.Stat(rawURL); err == nil && !fi.IsDir() && strings.HasSuffix(strings.ToLower(rawURL), ".torrent") {
		kind = store.KindTorrent
	}
	s := a.mgr.Settings()
	res := &ProbeResult{Kind: kind, Size: -1}
	if kind != store.KindHTTP {
		res.Category = map[string]string{store.KindTorrent: "Torrents"}[kind]
		if res.Category == "" {
			res.Category = "Vidéos"
		}
		res.Dir = s.DirFor(res.Category)
		return res, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := engine.Probe(ctx, a.mgr.HTTPClient(), a.mgr.EngineConfig(), rawURL, a.mgr.AuthHeaders(rawURL, headers))
	if err != nil {
		return nil, err
	}
	res.FileName = info.FileName
	res.Size = info.Size
	res.Resumable = info.Resumable
	res.ContentType = info.ContentType
	res.FinalURL = info.FinalURL
	res.Category = s.CategoryFor(info.FileName)
	res.Dir = s.DirFor(res.Category)
	// A page that is actually HTML: probably a video page -> let the UI propose yt-dlp.
	if strings.HasPrefix(info.ContentType, "text/html") {
		res.Kind = "page"
	}
	return res, nil
}

// Add creates a download. If start is true it starts (or is queued) immediately.
func (a *API) Add(req core.AddRequest) (*store.Item, error) {
	it, err := a.mgr.Add(req)
	if err != nil {
		return nil, err
	}
	s := a.mgr.Settings()
	if req.Start && s.ShowProgressWin && (req.Queue == "" || req.Queue == core.MainQueue) {
		a.ui.OpenProgress(it.ID)
	}
	return it, nil
}

// AddBatch adds several downloads at once ("download all").
func (a *API) AddBatch(reqs []core.AddRequest) (int, error) {
	n := 0
	var lastErr error
	for _, r := range reqs {
		if _, err := a.mgr.Add(r); err != nil {
			lastErr = err
			continue
		}
		n++
	}
	if n == 0 && lastErr != nil {
		return 0, lastErr
	}
	return n, nil
}

// Pending returns the request attached to a dialog window.
func (a *API) Pending(token string) (*PendingAdd, error) { return a.ui.TakePending(token) }

// DonePending releases a dialog token.
func (a *API) DonePending(token string) { a.ui.DropPending(token) }

// OpenAddDialog opens the add-URL dialog (optionally pre-filled).
func (a *API) OpenAddDialog(rawURL string) {
	a.ui.OpenAdd(core.AddRequest{URL: rawURL, Start: true}, nil, false)
}

// OpenProgress opens the progress window of a download.
func (a *API) OpenProgress(id string) { a.ui.OpenProgress(id) }

// ShowMain focuses the main window.
func (a *API) ShowMain() { a.ui.ShowMain() }

// Start starts downloads now.
func (a *API) Start(ids []string) error {
	var last error
	for _, id := range ids {
		if err := a.mgr.Start(id); err != nil {
			last = err
		}
	}
	return last
}

// Pause pauses downloads.
func (a *API) Pause(ids []string) {
	for _, id := range ids {
		go a.mgr.Pause(id)
	}
}

// Enqueue queues downloads (they start when their queue allows).
func (a *API) Enqueue(ids []string) {
	for _, id := range ids {
		_ = a.mgr.Enqueue(id)
	}
}

// Remove deletes downloads (and optionally their files).
func (a *API) Remove(ids []string, deleteFiles bool) {
	for _, id := range ids {
		_ = a.mgr.Remove(id, deleteFiles)
	}
}

// RemoveCompleted removes all finished items from the list (files are kept).
func (a *API) RemoveCompleted() int {
	n := 0
	for _, it := range a.mgr.List() {
		if it.Status == store.StatusCompleted {
			_ = a.mgr.Remove(it.ID, false)
			n++
		}
	}
	return n
}

// Redownload restarts a download from zero.
func (a *API) Redownload(id string) error { return a.mgr.Redownload(id) }

// StartAll resumes everything.
func (a *API) StartAll() { a.mgr.StartAll() }

// PauseAll pauses everything.
func (a *API) PauseAll() { go a.mgr.PauseAll() }

// UpdateURL refreshes the address of a download (expired link).
func (a *API) UpdateURL(id, rawURL string) error { return a.mgr.UpdateURL(id, rawURL, nil) }

// SetItemOptions changes per-download options (connections, speedLimit...).
func (a *API) SetItemOptions(id string, opts map[string]any) error {
	return a.mgr.SetItemOptions(id, opts)
}

// SetSpeedLimit sets the global speed limit in bytes/s (0 = unlimited).
func (a *API) SetSpeedLimit(bps int64) error { return a.mgr.SetSpeedLimit(bps) }

// Queues returns the queues.
func (a *API) Queues() []core.QueueState { return a.mgr.Queues() }

// SaveQueue creates/updates a queue.
func (a *API) SaveQueue(q core.QueueState) error { return a.mgr.SaveQueue(q) }

// DeleteQueue removes a queue.
func (a *API) DeleteQueue(name string) error { return a.mgr.DeleteQueue(name) }

// StartQueue starts a queue.
func (a *API) StartQueue(name string) { a.mgr.StartQueue(name) }

// StopQueue stops a queue.
func (a *API) StopQueue(name string) { go a.mgr.StopQueue(name) }

// MoveToQueue moves downloads to a queue.
func (a *API) MoveToQueue(ids []string, queue string) { a.mgr.MoveToQueue(ids, queue) }

// OpenFile opens a completed file with the default application.
func (a *API) OpenFile(id string) error {
	it, err := a.mgr.Get(id)
	if err != nil {
		return err
	}
	if it.Path == "" {
		return errors.New("fichier pas encore disponible")
	}
	return a.ui.app.Browser.OpenFile(it.Path)
}

// OpenFolder shows the file in the system file manager.
func (a *API) OpenFolder(id string) error {
	it, err := a.mgr.Get(id)
	if err != nil {
		return err
	}
	p := it.Path
	if p == "" {
		return revealPath(it.Dir, false)
	}
	if _, err := os.Stat(p); err != nil {
		p += engine.PartExt
	}
	return revealPath(p, true)
}

// OpenDir opens a folder.
func (a *API) OpenDir(dir string) error { return revealPath(dir, false) }

// OpenURL opens a web page in the default browser.
func (a *API) OpenURL(u string) error { return a.ui.app.Browser.OpenURL(u) }

// ChooseFolder shows a folder picker.
func (a *API) ChooseFolder(initial string) (string, error) {
	d := a.ui.app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).SetTitle("Choisir un dossier")
	if initial != "" {
		d.SetDirectory(initial)
	}
	return d.PromptForSingleSelection()
}

// ChooseFile shows a file picker; pattern like "*.torrent".
func (a *API) ChooseFile(title, filterName, pattern string) (string, error) {
	d := a.ui.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle(title)
	if pattern != "" {
		d.AddFilter(filterName, pattern)
	}
	return d.PromptForSingleSelection()
}

// ExportList writes all URLs to a text file.
func (a *API) ExportList() (string, error) {
	p, err := a.ui.app.Dialog.SaveFile().SetFilename("fluxdm-liste.txt").AddFilter("Texte", "*.txt").PromptForSingleSelection()
	if err != nil || p == "" {
		return "", err
	}
	var b strings.Builder
	for _, it := range a.mgr.List() {
		b.WriteString(it.URL)
		b.WriteByte('\n')
	}
	return p, os.WriteFile(p, []byte(b.String()), 0o644)
}

// ImportList reads URLs from a text file and opens the batch dialog.
func (a *API) ImportList() (int, error) {
	p, err := a.ChooseFile("Importer une liste", "Texte", "*.txt")
	if err != nil || p == "" {
		return 0, err
	}
	return a.importFile(p), nil
}

func (a *API) importFile(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	var reqs []core.AddRequest
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		reqs = append(reqs, core.AddRequest{URL: line})
	}
	if len(reqs) > 0 {
		a.ui.OpenAdd(core.AddRequest{}, reqs, false)
	}
	return len(reqs)
}

// ClipboardText returns the clipboard text (to pre-fill the add dialog).
func (a *API) ClipboardText() string {
	t, _ := a.ui.app.Clipboard.Text()
	t = strings.TrimSpace(t)
	if looksDownloadable(t, nil) || strings.HasPrefix(t, "http") {
		return t
	}
	return ""
}

// DiskFree returns free bytes on the volume of dir (-1 unknown).
func (a *API) DiskFree(dir string) int64 { return diskFree(dir) }

// Stats returns global counters for the status bar.
func (a *API) Stats() map[string]any {
	items := a.mgr.List()
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Status]++
	}
	return map[string]any{
		"total":      len(items),
		"active":     a.mgr.ActiveCount(),
		"counts":     counts,
		"speedLimit": a.mgr.Limiter().Rate(),
	}
}

func revealPath(p string, selectFile bool) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if selectFile {
			cmd = exec.Command("explorer", "/select,", filepath.Clean(p))
		} else {
			cmd = exec.Command("explorer", filepath.Clean(p))
		}
	case "darwin":
		if selectFile {
			cmd = exec.Command("open", "-R", p)
		} else {
			cmd = exec.Command("open", p)
		}
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(p))
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("impossible d'ouvrir l'explorateur : %w", err)
	}
	go cmd.Wait()
	return nil
}
