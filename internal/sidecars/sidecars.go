// Package sidecars downloads and updates the external tools FluxDM relies on
// (yt-dlp, ffmpeg, deno) on first use, so the installer stays small.
package sidecars

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fluxdm/internal/engine"
)

// Tool names.
const (
	YtDlp  = "yt-dlp"
	FFmpeg = "ffmpeg"
	Deno   = "deno"
)

// Status describes an installed tool.
type Status struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Managed   bool   `json:"managed"` // installed by FluxDM (can be updated)
}

// Progress is reported while a tool downloads.
type Progress struct {
	Tool       string `json:"tool"`
	Downloaded int64  `json:"downloaded"`
	Size       int64  `json:"size"`
	Done       bool   `json:"done"`
	Error      string `json:"error,omitempty"`
}

// Manager installs tools into Dir.
type Manager struct {
	Dir        string
	Client     func() *http.Client
	Config     func() engine.Config
	OnProgress func(Progress)
	HideWindow func(*exec.Cmd)

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done chan struct{}
	path string
	err  error
}

// New creates a tool manager.
func New(dir string, client func() *http.Client, cfg func() engine.Config) *Manager {
	return &Manager{Dir: dir, Client: client, Config: cfg, inflight: map[string]*call{}}
}

func exeName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

// managedPath is where FluxDM installs the tool.
func (m *Manager) managedPath(tool string) string {
	return filepath.Join(m.Dir, exeName(tool))
}

// Path returns the tool path (FluxDM copy first, then PATH), or "".
func (m *Manager) Path(tool string) string {
	if p := m.managedPath(tool); fileExists(p) {
		return p
	}
	if p, err := exec.LookPath(tool); err == nil {
		return p
	}
	return ""
}

// BinDir returns the folder containing managed tools (prepend to PATH so
// yt-dlp finds deno and ffmpeg).
func (m *Manager) BinDir() string { return m.Dir }

// Env returns the environment for running tools with Dir first in PATH.
func (m *Manager) Env() []string {
	env := os.Environ()
	sep := string(os.PathListSeparator)
	for i, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && strings.EqualFold(k, "PATH") {
			env[i] = k + "=" + m.Dir + sep + v
			return env
		}
	}
	return append(env, "PATH="+m.Dir)
}

// Ensure returns the tool path, downloading it if needed.
func (m *Manager) Ensure(ctx context.Context, tool string) (string, error) {
	if p := m.Path(tool); p != "" {
		return p, nil
	}
	return m.install(ctx, tool)
}

// Update re-downloads a managed tool (yt-dlp changes very often).
func (m *Manager) Update(ctx context.Context, tool string) error {
	_, err := m.install(ctx, tool)
	return err
}

func (m *Manager) install(ctx context.Context, tool string) (string, error) {
	m.mu.Lock()
	if c, ok := m.inflight[tool]; ok {
		m.mu.Unlock()
		select {
		case <-c.done:
			return c.path, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := &call{done: make(chan struct{})}
	m.inflight[tool] = c
	m.mu.Unlock()

	c.path, c.err = m.doInstall(ctx, tool)
	if m.OnProgress != nil {
		p := Progress{Tool: tool, Done: true}
		if c.err != nil {
			p.Error = c.err.Error()
		}
		m.OnProgress(p)
	}
	close(c.done)
	m.mu.Lock()
	delete(m.inflight, tool)
	m.mu.Unlock()
	return c.path, c.err
}

type source struct {
	url     string
	archive bool     // zip
	files   []string // base names to extract from the archive
}

func sourceFor(tool string) (source, error) {
	switch tool + "/" + runtime.GOOS {
	case "yt-dlp/windows":
		return source{url: "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"}, nil
	case "yt-dlp/darwin":
		return source{url: "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"}, nil
	case "yt-dlp/linux":
		return source{url: "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux"}, nil
	case "ffmpeg/windows":
		return source{url: "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip", archive: true, files: []string{"ffmpeg.exe", "ffprobe.exe"}}, nil
	case "ffmpeg/darwin":
		return source{url: "https://evermeet.cx/ffmpeg/getrelease/zip", archive: true, files: []string{"ffmpeg"}}, nil
	case "ffmpeg/linux":
		return source{url: "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz"}, errors.New("installez ffmpeg avec votre gestionnaire de paquets")
	case "deno/windows":
		return source{url: "https://github.com/denoland/deno/releases/latest/download/deno-x86_64-pc-windows-msvc.zip", archive: true, files: []string{"deno.exe"}}, nil
	case "deno/darwin":
		arch := "x86_64"
		if runtime.GOARCH == "arm64" {
			arch = "aarch64"
		}
		return source{url: "https://github.com/denoland/deno/releases/latest/download/deno-" + arch + "-apple-darwin.zip", archive: true, files: []string{"deno"}}, nil
	case "deno/linux":
		return source{url: "https://github.com/denoland/deno/releases/latest/download/deno-x86_64-unknown-linux-gnu.zip", archive: true, files: []string{"deno"}}, nil
	}
	return source{}, fmt.Errorf("outil inconnu : %s", tool)
}

func (m *Manager) doInstall(ctx context.Context, tool string) (string, error) {
	src, err := sourceFor(tool)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return "", err
	}
	tmpDir := filepath.Join(m.Dir, ".tmp-"+tool)
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	cfg := m.Config()
	cfg.SpeedLimiter = nil
	d := engine.NewDownload(m.Client(), cfg, engine.Snapshot{URL: src.url, Dir: tmpDir, FileName: "download.bin"})
	d.OnProgress = func(p engine.Progress) {
		if m.OnProgress != nil {
			m.OnProgress(Progress{Tool: tool, Downloaded: p.Downloaded, Size: p.Size})
		}
	}
	if err := d.Run(ctx); err != nil {
		return "", fmt.Errorf("téléchargement de %s : %w", tool, err)
	}
	got := d.Snapshot().Path

	dst := m.managedPath(tool)
	if src.archive {
		if err := extractZip(got, m.Dir, src.files); err != nil {
			return "", fmt.Errorf("extraction de %s : %w", tool, err)
		}
	} else {
		_ = os.Remove(dst)
		if err := os.Rename(got, dst); err != nil {
			return "", err
		}
	}
	if runtime.GOOS != "windows" {
		for _, f := range append(src.files, exeName(tool)) {
			_ = os.Chmod(filepath.Join(m.Dir, f), 0o755)
		}
	}
	if !fileExists(dst) {
		return "", fmt.Errorf("%s introuvable après installation", tool)
	}
	return dst, nil
}

func extractZip(zipPath, dir string, want []string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	found := 0
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		match := false
		for _, w := range want {
			if strings.EqualFold(base, w) {
				match = true
			}
		}
		if !match || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out := filepath.Join(dir, base)
		tmp := out + ".new"
		w, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
		_ = os.Remove(out)
		if err := os.Rename(tmp, out); err != nil {
			return err
		}
		found++
	}
	if found == 0 {
		return errors.New("fichier attendu absent de l'archive")
	}
	return nil
}

// Version runs the tool and returns its first version line.
func (m *Manager) Version(tool string) string {
	p := m.Path(tool)
	if p == "" {
		return ""
	}
	arg := "--version"
	if tool == FFmpeg {
		arg = "-version"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, arg)
	cmd.Env = m.Env()
	if m.HideWindow != nil {
		m.HideWindow(cmd)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if tool == FFmpeg {
		f := strings.Fields(line)
		if len(f) >= 3 {
			return f[2]
		}
	}
	if tool == Deno {
		if f := strings.Fields(line); len(f) >= 2 {
			return f[1]
		}
	}
	return line
}

// Statuses lists all tools.
func (m *Manager) Statuses() []Status {
	var out []Status
	for _, t := range []string{YtDlp, FFmpeg, Deno} {
		p := m.Path(t)
		out = append(out, Status{Name: t, Installed: p != "", Path: p, Version: m.Version(t), Managed: p == m.managedPath(t)})
	}
	return out
}

// LatestYtdlp returns the latest yt-dlp release tag from GitHub.
func (m *Manager) LatestYtdlp(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "FluxDM")
	resp, err := m.Client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub : %s", resp.Status)
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Tag, nil
}

// AutoUpdate updates yt-dlp when a newer release exists (only managed copies).
func (m *Manager) AutoUpdate(ctx context.Context) error {
	if !fileExists(m.managedPath(YtDlp)) {
		return nil
	}
	latest, err := m.LatestYtdlp(ctx)
	if err != nil {
		return err
	}
	if cur := m.Version(YtDlp); cur != "" && cur == latest {
		return nil
	}
	return m.Update(ctx, YtDlp)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
