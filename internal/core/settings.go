package core

import (
	"os"
	"path/filepath"
	"strings"
)

// Category groups files by extension into their own folder (like IDM).
type Category struct {
	Name string   `json:"name"`
	Exts []string `json:"exts"`
	Dir  string   `json:"dir"` // absolute; empty = <DownloadDir>/<Name>
}

// Settings are the user preferences (persisted as JSON).
type Settings struct {
	DownloadDir         string     `json:"downloadDir"`
	MaxConns            int        `json:"maxConns"`            // connections per file
	MaxActive           int        `json:"maxActive"`           // simultaneous downloads in the main queue
	SpeedLimit          int64      `json:"speedLimit"`          // global bytes/s, 0 = unlimited
	Proxy               string     `json:"proxy"`               // "", http://..., socks5://...
	AllowHTTP2          bool       `json:"allowHttp2"`          // off = one TCP connection per segment (faster)
	InsecureTLS         bool       `json:"insecureTls"`         // accept invalid certificates
	UserAgent           string     `json:"userAgent"`           // empty = default browser UA
	Categories          []Category `json:"categories"`          //
	CaptureExts         []string   `json:"captureExts"`         // extensions captured from the browser
	CaptureMinSize      int64      `json:"captureMinSize"`      // do not capture files smaller than this (bytes)
	SkipSites           []string   `json:"skipSites"`           // never capture from these hosts
	ClipboardMonitor    bool       `json:"clipboardMonitor"`    // offer to download copied URLs
	ShowAddDialog       bool       `json:"showAddDialog"`       // confirm dialog for captured downloads
	ShowDoneDialog      bool       `json:"showDoneDialog"`      // "download complete" dialog
	AntivirusPath       string     `json:"antivirusPath"`       // scanner executable ("" = none)
	AntivirusArgs       string     `json:"antivirusArgs"`       // args, "{file}" is replaced by the path
	Language            string     `json:"language"`            // "fr" | "en"
	Theme               string     `json:"theme"`               // "system" | "light" | "dark"
	HLSOutput           string     `json:"hlsOutput"`           // "mp4" | "mkv" | "ts"
	HLSConcurrency      int        `json:"hlsConcurrency"`      // parallel segments for HLS/DASH
	VideoQuality        string     `json:"videoQuality"`        // "best" | "1080" | "720" | "480" | "audio"
	AutoUpdateTools     bool       `json:"autoUpdateTools"`     // keep yt-dlp/rules up to date
	YtdlpCookiesBrowser string     `json:"ytdlpCookiesBrowser"` // "", chrome, opera, edge, brave, firefox (age-restricted videos)
	Credentials         []Cred     `json:"credentials"`         // per-site logins
	TorrentPort         int        `json:"torrentPort"`         // 0 = random
	TorrentSeed         bool       `json:"torrentSeed"`         // keep seeding after completion
	StartMinimized      bool       `json:"startMinimized"`      //
	LaunchAtStartup     bool       `json:"launchAtStartup"`     //
	ShowProgressWin     bool       `json:"showProgressWin"`     // open a progress window when a download starts
	CloseToTray         bool       `json:"closeToTray"`         // closing the main window keeps FluxDM in the tray
	VideoButton         bool       `json:"videoButton"`         // show the floating "download video" button in pages
	SettingsVersion     int        `json:"settingsVersion"`     // for migrations
	RulesURL            string     `json:"rulesUrl"`            // hosters.json update URL ("" = built-in only)
	UpdateRepo          string     `json:"updateRepo"`          // GitHub "owner/repo" for app updates
}

// Cred is a login for a site (Basic/Digest/NTLM/FTP).
type Cred struct {
	Host     string `json:"host"` // host or *.domain
	User     string `json:"user"`
	Password string `json:"password"`
}

// DefaultSettings returns the defaults used on first launch.
func DefaultSettings() Settings {
	home, _ := os.UserHomeDir()
	dl := filepath.Join(home, "Downloads", "FluxDM")
	return Settings{
		DownloadDir: dl,
		MaxConns:    8,
		MaxActive:   3,
		Categories: []Category{
			{Name: "Compressés", Exts: split("zip rar 7z gz tgz tar bz2 xz zst iso cab arj lzh")},
			{Name: "Documents", Exts: split("pdf doc docx xls xlsx ppt pptx odt ods odp txt rtf epub csv")},
			{Name: "Musique", Exts: split("mp3 m4a aac flac wav ogg opus wma aif aiff mid")},
			{Name: "Programmes", Exts: split("exe msi msix appx dmg pkg apk deb rpm appimage jar bat")},
			{Name: "Vidéos", Exts: split("mp4 mkv avi mov webm flv wmv ts m4v 3gp mpg mpeg ogv vob")},
			{Name: "Images", Exts: split("jpg jpeg png gif webp bmp tif tiff svg heic avif")},
			{Name: "Torrents", Exts: nil},
		},
		CaptureExts:      split("3gp 7z aac ace aif apk arj asf avi bin bz2 deb dmg exe flac flv gz gzip img iso jar lzh m4a m4v mkv mov mp3 mp4 mpeg mpg msi msu ogg ogv opus pdf pkg ppt pptx rar rpm sea sit tar tgz tif ts wav webm wma wmv xz zip zst doc docx xls xlsx epub torrent"),
		CaptureMinSize:   0,
		ShowAddDialog:    true,
		ShowDoneDialog:   true,
		ClipboardMonitor: false,
		Language:         "fr",
		Theme:            "system",
		HLSOutput:        "mp4",
		HLSConcurrency:   8,
		VideoQuality:     "best",
		AutoUpdateTools:  true,
		ShowProgressWin:  true,
		CloseToTray:      true,
		VideoButton:      true,
		SettingsVersion:  1,
	}
}

func split(s string) []string { return strings.Fields(s) }

func (s *Settings) normalize() {
	d := DefaultSettings()
	if s.DownloadDir == "" {
		s.DownloadDir = d.DownloadDir
	}
	if s.MaxConns <= 0 {
		s.MaxConns = d.MaxConns
	}
	if s.MaxConns > 32 {
		s.MaxConns = 32
	}
	if s.MaxActive <= 0 {
		s.MaxActive = d.MaxActive
	}
	if len(s.Categories) == 0 {
		s.Categories = d.Categories
	}
	if s.Language == "" {
		s.Language = d.Language
	}
	if s.Theme == "" {
		s.Theme = d.Theme
	}
	if s.HLSOutput == "" {
		s.HLSOutput = d.HLSOutput
	}
	if s.HLSConcurrency <= 0 {
		s.HLSConcurrency = d.HLSConcurrency
	}
	if s.VideoQuality == "" {
		s.VideoQuality = d.VideoQuality
	}
}

// CategoryFor returns the category name for a file name ("" = general).
func (s *Settings) CategoryFor(fileName string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if ext == "" {
		return ""
	}
	if ext == "torrent" {
		return "Torrents"
	}
	for _, c := range s.Categories {
		for _, e := range c.Exts {
			if strings.EqualFold(e, ext) {
				return c.Name
			}
		}
	}
	return ""
}

// DirFor returns the destination folder for a category.
func (s *Settings) DirFor(category string) string {
	if category == "" {
		return s.DownloadDir
	}
	for _, c := range s.Categories {
		if c.Name == category {
			if c.Dir != "" {
				return c.Dir
			}
			return filepath.Join(s.DownloadDir, c.Name)
		}
	}
	return s.DownloadDir
}

// CredFor finds stored credentials for host.
func (s *Settings) CredFor(host string) *Cred {
	host = strings.ToLower(host)
	for i := range s.Credentials {
		h := strings.ToLower(s.Credentials[i].Host)
		if h == host || (strings.HasPrefix(h, "*.") && strings.HasSuffix(host, h[1:])) {
			return &s.Credentials[i]
		}
	}
	return nil
}
