package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
)

// YtdlpHosts are page hosts sent to yt-dlp (YouTube and other big sites).
var YtdlpHosts = []string{
	"youtube.com", "youtu.be", "youtube-nocookie.com", "vimeo.com", "dailymotion.com", "dai.ly",
	"twitter.com", "x.com", "facebook.com", "fb.watch", "instagram.com", "tiktok.com", "twitch.tv",
	"soundcloud.com", "reddit.com", "v.redd.it", "bilibili.com", "bilibili.tv", "nicovideo.jp", "vk.com",
	"vkvideo.ru", "ok.ru", "rumble.com", "streamable.com", "bandcamp.com", "mixcloud.com", "odysee.com",
	"bitchute.com", "9gag.com", "imgur.com", "pinterest.com", "linkedin.com", "tumblr.com", "ted.com",
	"arte.tv", "france.tv", "rutube.ru", "coub.com", "kick.com", "threads.net", "bsky.app", "vimeo.com",
	"loom.com", "archive.org", "peertube.tv", "veoh.com", "metacafe.com", "zdf.de", "ardmediathek.de",
	"bbc.co.uk", "cnn.com", "nytimes.com", "espn.com", "weibo.com", "douyin.com", "kuaishou.com",
	"iq.com", "niconico.jp", "naver.com", "afreecatv.com", "sooplive.co.kr", "trovo.live", "dlive.tv",
}

// IsYtdlpHost reports whether a page URL should be handled by yt-dlp.
func IsYtdlpHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	h = strings.TrimPrefix(h, "m.")
	for _, s := range YtdlpHosts {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	return false
}

// Format is one yt-dlp format.
type Format struct {
	ID       string  `json:"id"`
	Ext      string  `json:"ext"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      float64 `json:"fps"`
	VCodec   string  `json:"vcodec"`
	ACodec   string  `json:"acodec"`
	Filesize int64   `json:"filesize"`
	TBR      float64 `json:"tbr"`
	Note     string  `json:"note"`
	Protocol string  `json:"protocol"`
}

// Entry is a playlist entry.
type Entry struct {
	URL      string  `json:"url"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
}

// VideoInfo is what the quality dialog displays.
type VideoInfo struct {
	Source    string      `json:"source"` // ytdlp | hls | dash | direct
	Title     string      `json:"title"`
	Thumbnail string      `json:"thumbnail,omitempty"`
	Duration  float64     `json:"duration"`
	Extractor string      `json:"extractor,omitempty"`
	Formats   []Format    `json:"formats"`
	Subtitles []string    `json:"subtitles,omitempty"`
	Playlist  bool        `json:"playlist"`
	Entries   []Entry     `json:"entries,omitempty"`
	Variants  []Variant   `json:"variants,omitempty"` // HLS
	Dash      []DashRep   `json:"dash,omitempty"`
	Audio     []Rendition `json:"audio,omitempty"`
	Live      bool        `json:"live"`
}

func (rt *Runtime) ytdlpBase(ctx context.Context, it *store.Item, headers map[string]string, pageURL string) (string, []string, func(), error) {
	bin, err := rt.Tools.Ensure(ctx, sidecars.YtDlp)
	if err != nil {
		return "", nil, nil, fmt.Errorf("yt-dlp indisponible : %w", err)
	}
	// Deno is required for YouTube (yt-dlp >= 2025.11); best effort.
	if IsYouTube(pageURL) {
		_, _ = rt.Tools.Ensure(ctx, sidecars.Deno)
	}
	s := rt.M.Settings()
	args := []string{"--no-colors", "--encoding", "utf-8", "--no-warnings"}
	if s.Proxy != "" {
		args = append(args, "--proxy", s.Proxy)
	}
	cleanup := func() {}
	if ua := headers["User-Agent"]; ua != "" {
		args = append(args, "--user-agent", ua)
	}
	if ref := headers["Referer"]; ref != "" {
		args = append(args, "--referer", ref)
	}
	if s.YtdlpCookiesBrowser != "" {
		args = append(args, "--cookies-from-browser", s.YtdlpCookiesBrowser)
	} else if c := headers["Cookie"]; c != "" {
		if f, err := writeCookieFile(c, pageURL); err == nil {
			args = append(args, "--cookies", f)
			cleanup = func() { os.Remove(f) }
		}
	}
	if ff := rt.Tools.Path(sidecars.FFmpeg); ff != "" {
		args = append(args, "--ffmpeg-location", ff)
	}
	if s.InsecureTLS {
		args = append(args, "--no-check-certificates")
	}
	_ = it
	return bin, args, cleanup, nil
}

// IsYouTube reports whether the URL is a YouTube page.
func IsYouTube(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return strings.Contains(h, "youtube.com") || strings.Contains(h, "youtu.be") || strings.Contains(h, "youtube-nocookie.com")
}

// writeCookieFile converts "a=b; c=d" into a Netscape cookies.txt for the page domain.
func writeCookieFile(cookie, pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("domaine inconnu")
	}
	domain := "." + strings.TrimPrefix(u.Hostname(), "www.")
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	exp := time.Now().Add(24 * time.Hour).Unix()
	for _, part := range strings.Split(cookie, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || k == "" {
			continue
		}
		fmt.Fprintf(&b, "%s\tTRUE\t/\tTRUE\t%d\t%s\t%s\n", domain, exp, k, v)
	}
	f, err := os.CreateTemp("", "fluxdm-cookies-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return f.Name(), err
}

func (rt *Runtime) command(ctx context.Context, bin string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(rt.Tools.Env(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	core.HideWindow(cmd)
	return cmd
}

// AnalyzeYtdlp lists formats (or playlist entries) of a page.
func (rt *Runtime) AnalyzeYtdlp(ctx context.Context, pageURL string, headers map[string]string, playlist bool) (*VideoInfo, error) {
	bin, args, cleanup, err := rt.ytdlpBase(ctx, nil, headers, pageURL)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	args = append(args, "-J", "--skip-download")
	if playlist {
		args = append(args, "--flat-playlist", "--yes-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, "--", pageURL)
	cmd := rt.command(ctx, bin, args)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, ytdlpError(stderr.String(), err)
	}
	var raw struct {
		Type      string  `json:"_type"`
		Title     string  `json:"title"`
		Thumbnail string  `json:"thumbnail"`
		Duration  float64 `json:"duration"`
		Extractor string  `json:"extractor_key"`
		IsLive    bool    `json:"is_live"`
		Formats   []struct {
			ID       string  `json:"format_id"`
			Ext      string  `json:"ext"`
			Width    int     `json:"width"`
			Height   int     `json:"height"`
			FPS      float64 `json:"fps"`
			VCodec   string  `json:"vcodec"`
			ACodec   string  `json:"acodec"`
			Filesize int64   `json:"filesize"`
			Approx   int64   `json:"filesize_approx"`
			TBR      float64 `json:"tbr"`
			Note     string  `json:"format_note"`
			Protocol string  `json:"protocol"`
		} `json:"formats"`
		Subtitles map[string]any `json:"subtitles"`
		Entries   []struct {
			URL      string  `json:"url"`
			WebURL   string  `json:"webpage_url"`
			Title    string  `json:"title"`
			Duration float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("réponse yt-dlp illisible : %w", err)
	}
	info := &VideoInfo{Source: "ytdlp", Title: raw.Title, Thumbnail: raw.Thumbnail, Duration: raw.Duration, Extractor: raw.Extractor, Live: raw.IsLive}
	for _, f := range raw.Formats {
		if f.Protocol == "mhtml" || strings.Contains(f.Note, "storyboard") {
			continue
		}
		size := f.Filesize
		if size == 0 {
			size = f.Approx
		}
		info.Formats = append(info.Formats, Format{ID: f.ID, Ext: f.Ext, Width: f.Width, Height: f.Height, FPS: f.FPS,
			VCodec: f.VCodec, ACodec: f.ACodec, Filesize: size, TBR: f.TBR, Note: f.Note, Protocol: f.Protocol})
	}
	for lang := range raw.Subtitles {
		info.Subtitles = append(info.Subtitles, lang)
	}
	if raw.Type == "playlist" {
		info.Playlist = true
		for _, e := range raw.Entries {
			u := e.WebURL
			if u == "" {
				u = e.URL
			}
			info.Entries = append(info.Entries, Entry{URL: u, Title: e.Title, Duration: e.Duration})
		}
	}
	return info, nil
}

func ytdlpError(stderr string, err error) error {
	var lines []string
	for _, l := range strings.Split(stderr, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "ERROR:") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(l, "ERROR:")))
		}
	}
	if len(lines) > 0 {
		return errors.New(strings.Join(lines, " / "))
	}
	s := strings.TrimSpace(stderr)
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	if s != "" {
		return fmt.Errorf("yt-dlp : %s", s)
	}
	return fmt.Errorf("yt-dlp : %w", err)
}

// formatSelector builds the -f expression from options / settings.
func formatSelector(it *store.Item, s core.Settings) (string, bool) {
	if f := core.OptString(it.Options, "format"); f != "" {
		return f, false
	}
	h, audio := maxHeightFor(it, s)
	if audio {
		return "ba/b", true
	}
	if h > 0 {
		return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/bv*+ba/b", h, h), false
	}
	return "bv*+ba/b", false
}

const progressPrefix = "[FDM]"

// RunYtdlp is the runner for store.KindYtdlp.
func (rt *Runtime) RunYtdlp(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	bin, args, cleanup, err := rt.ytdlpBase(ctx, it, it.Headers, it.URL)
	if err != nil {
		return err
	}
	defer cleanup()
	if rt.Tools.Path(sidecars.FFmpeg) == "" {
		if ff, err := rt.Tools.Ensure(ctx, sidecars.FFmpeg); err == nil {
			args = append(args, "--ffmpeg-location", ff)
		}
	}
	s := m.Settings()
	if err := os.MkdirAll(it.Dir, 0o755); err != nil {
		return err
	}
	sel, audio := formatSelector(it, s)
	outFmt := core.OptString(it.Options, "output")
	if outFmt == "" {
		outFmt = s.HLSOutput
	}
	args = append(args, "--no-playlist", "--newline", "--continue", "--no-mtime", "-f", sel)
	if audio {
		af := outFmt
		if af != "mp3" && af != "m4a" && af != "opus" && af != "flac" && af != "wav" {
			af = "mp3"
		}
		args = append(args, "-x", "--audio-format", af, "--audio-quality", "0")
	} else {
		mf := outFmt
		if mf != "mkv" && mf != "webm" {
			mf = "mp4"
		}
		args = append(args, "--merge-output-format", mf)
	}
	if subs := core.OptString(it.Options, "subtitles"); subs != "" {
		args = append(args, "--write-subs", "--sub-langs", subs, "--embed-subs")
	}
	if core.OptBool(it.Options, "thumbnail") {
		args = append(args, "--embed-thumbnail")
	}
	conc := s.HLSConcurrency
	if conc <= 0 {
		conc = 8
	}
	args = append(args, "--concurrent-fragments", strconv.Itoa(conc))
	if lim := m.Limiter().Rate(); lim > 0 {
		args = append(args, "--limit-rate", strconv.FormatInt(lim, 10))
	}
	name := "%(title).180B [%(id)s].%(ext)s"
	if it.FileName != "" {
		name = strings.TrimSuffix(it.FileName, filepath.Ext(it.FileName)) + ".%(ext)s"
		name = strings.ReplaceAll(name, "%", "%%")
		name = strings.ReplaceAll(name, "%%(ext)s", "%(ext)s")
	}
	tmp := m.TempDir(it.ID)
	_ = os.MkdirAll(tmp, 0o755)
	args = append(args,
		"-P", it.Dir, "-P", "temp:"+tmp, "-o", name,
		"--progress-template", "download:"+progressPrefix+" %(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s|%(progress.fragment_index)s|%(progress.fragment_count)s",
		"--print", "before_dl:[FDMTITLE] %(title)s",
		"--print", "after_move:[FDMFILE] %(filepath)s",
		"--no-simulate",
		"--", it.URL,
	)
	cmd := rt.command(ctx, bin, args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	var finalPath string
	var phaseBase, lastDone, lastTotal int64
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "[FDMTITLE] "):
			t := strings.TrimPrefix(line, "[FDMTITLE] ")
			if it.Title == "" && t != "" && t != "NA" {
				rep.Title(t)
			}
		case strings.HasPrefix(line, "[FDMFILE] "):
			finalPath = strings.TrimPrefix(line, "[FDMFILE] ")
		case strings.HasPrefix(line, progressPrefix):
			f := strings.Split(strings.TrimSpace(strings.TrimPrefix(line, progressPrefix)), "|")
			if len(f) < 7 {
				continue
			}
			done := num(f[0])
			total := num(f[1])
			if total <= 0 {
				total = num(f[2])
			}
			// a new phase (audio after video) restarts the counters
			if done < lastDone/2 && lastTotal > 0 {
				phaseBase += lastTotal
			}
			lastDone, lastTotal = done, total
			p := core.Progress{Downloaded: phaseBase + done, Speed: numF(f[3]), Conns: conc, ETA: numF(f[4])}
			if total > 0 {
				p.Size = phaseBase + total
			} else {
				p.Size = -1
			}
			if p.ETA <= 0 {
				p.ETA = -1
			}
			p.Extra = map[string]any{"fragment": num(f[5]), "fragments": num(f[6])}
			rep.Progress(p)
		case strings.HasPrefix(line, "[Merger]"), strings.HasPrefix(line, "[ExtractAudio]"), strings.HasPrefix(line, "[VideoConvertor]"):
			rep.Status(store.StatusProcessing)
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ytdlpError(stderr.String(), err)
	}
	if finalPath != "" {
		var size int64
		if fi, err := os.Stat(finalPath); err == nil {
			size = fi.Size()
		}
		rep.Info(filepath.Base(finalPath), finalPath, size)
	}
	_ = os.RemoveAll(tmp)
	return nil
}

func num(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "None" {
		return 0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func numF(s string) float64 {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
