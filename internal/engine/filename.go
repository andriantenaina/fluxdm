package engine

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FileNameFromResponse derives a file name from Content-Disposition, the URL
// and the MIME type, in that order.
func FileNameFromResponse(rawURL, contentDisposition, contentType string) string {
	name := ""
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			name = params["filename"]
		}
	}
	if name == "" {
		if u, err := url.Parse(rawURL); err == nil {
			name = path.Base(u.Path)
			if name == "/" || name == "." {
				name = ""
			}
			if dec, err := url.PathUnescape(name); err == nil {
				name = dec
			}
		}
	}
	name = SanitizeFileName(name)
	if name == "" || name == "." || name == "/" {
		name = "download"
	}
	if filepath.Ext(name) == "" && contentType != "" {
		mt, _, _ := mime.ParseMediaType(contentType)
		if ext := extForMIME(mt); ext != "" {
			name += ext
		}
	}
	return name
}

var commonExt = map[string]string{
	"video/mp4":                     ".mp4",
	"video/webm":                    ".webm",
	"video/x-matroska":              ".mkv",
	"video/x-flv":                   ".flv",
	"video/mp2t":                    ".ts",
	"audio/mpeg":                    ".mp3",
	"audio/mp4":                     ".m4a",
	"audio/ogg":                     ".ogg",
	"application/zip":               ".zip",
	"application/pdf":               ".pdf",
	"application/x-rar-compressed":  ".rar",
	"application/x-7z-compressed":   ".7z",
	"application/x-msdownload":      ".exe",
	"application/x-bittorrent":      ".torrent",
	"application/vnd.apple.mpegurl": ".m3u8",
	"application/dash+xml":          ".mpd",
	"image/jpeg":                    ".jpg",
	"image/png":                     ".png",
}

func extForMIME(mt string) string {
	if e, ok := commonExt[mt]; ok {
		return e
	}
	if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// SanitizeFileName removes characters invalid on Windows/macOS.
func SanitizeFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		}
		if r < 32 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = name[:200-len(ext)] + ext
	}
	return name
}

// UniquePath returns a path in dir that does not exist yet: "a.zip", "a (1).zip", ...
func UniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	if !exists(p) && !exists(p+PartExt) {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if !exists(p) && !exists(p+PartExt) {
			return p
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
