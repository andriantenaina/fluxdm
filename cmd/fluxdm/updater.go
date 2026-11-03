package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fluxdm/internal/engine"
)

// UpdateInfo describes the latest GitHub release.
type UpdateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	URL     string `json:"url"`   // release page
	Asset   string `json:"asset"` // installer download URL for this OS
	Notes   string `json:"notes"`
}

func (a *API) latestRelease(ctx context.Context) (*UpdateInfo, error) {
	repo := strings.TrimSpace(a.mgr.Settings().UpdateRepo)
	if repo == "" {
		return nil, errors.New("aucun dépôt de mise à jour n'est configuré (Options > À propos)")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "FluxDM/"+a.ui.version)
	resp, err := a.mgr.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub : %s", resp.Status)
	}
	var r struct {
		Tag    string `json:"tag_name"`
		URL    string `json:"html_url"`
		Body   string `json:"body"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	info := &UpdateInfo{Current: a.ui.version, Latest: r.Tag, URL: r.URL, Notes: r.Body}
	info.Newer = newerVersion(r.Tag, a.ui.version)
	for _, as := range r.Assets {
		n := strings.ToLower(as.Name)
		if (runtime.GOOS == "windows" && strings.HasSuffix(n, ".exe") && strings.Contains(n, "setup")) ||
			(runtime.GOOS == "darwin" && strings.HasSuffix(n, ".dmg")) {
			info.Asset = as.URL
		}
	}
	return info, nil
}

// newerVersion compares "v1.2.3" style versions.
func newerVersion(latest, current string) bool {
	parse := func(s string) []int {
		s = strings.TrimPrefix(strings.TrimSpace(s), "v")
		if i := strings.IndexAny(s, "-+"); i >= 0 {
			s = s[:i]
		}
		var out []int
		for _, p := range strings.Split(s, ".") {
			n, _ := strconv.Atoi(p)
			out = append(out, n)
		}
		return out
	}
	l, c := parse(latest), parse(current)
	for i := 0; i < max(len(l), len(c)); i++ {
		var a, b int
		if i < len(l) {
			a = l[i]
		}
		if i < len(c) {
			b = c[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

// CheckUpdate looks for a newer FluxDM release.
func (a *API) CheckUpdate() (*UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return a.latestRelease(ctx)
}

// InstallUpdate downloads the installer of the latest release and runs it.
func (a *API) InstallUpdate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	info, err := a.latestRelease(ctx)
	if err != nil {
		return err
	}
	if !info.Newer {
		return errors.New("FluxDM est déjà à jour")
	}
	if info.Asset == "" {
		return a.ui.app.Browser.OpenURL(info.URL)
	}
	dir := filepath.Join(os.TempDir(), "fluxdm-update")
	_ = os.RemoveAll(dir)
	d := engine.NewDownload(a.mgr.HTTPClient(), a.mgr.EngineConfig(), engine.Snapshot{URL: info.Asset, Dir: dir})
	if err := d.Run(ctx); err != nil {
		return err
	}
	p := d.Snapshot().Path
	if runtime.GOOS == "windows" {
		cmd := exec.Command(p, "/S") // silent NSIS install; the installer restarts nothing, we relaunch below
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			a.ui.app.Quit()
		}()
		return nil
	}
	return exec.Command("open", p).Start()
}
