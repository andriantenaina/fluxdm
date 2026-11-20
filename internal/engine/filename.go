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
	return FixExtension(name, contentType)
}

// dynamicExts are server-script extensions that never describe the real file.
var dynamicExts = map[string]bool{".php": true, ".aspx": true, ".asp": true, ".jsp": true, ".cgi": true, ".pl": true, ".do": true, ".action": true, ".html": true, ".htm": true}

// FixExtension makes the extension consistent with the MIME type:
// "download.php" + video/mp4 -> "download.mp4", "file" + application/zip -> "file.zip",
// and removes doubled extensions ("a.mp4.mp4").
func FixExtension(name, contentType string) string {
	ext := strings.ToLower(filepath.Ext(name))
	mt, _, _ := mime.ParseMediaType(contentType)
	want := ""
	if mt != "" && mt != "application/octet-stream" && mt != "binary/octet-stream" && !strings.HasPrefix(mt, "text/") {
		want = extForMIME(mt)
	}
	switch {
	case want != "" && (ext == "" || dynamicExts[ext]):
		name = strings.TrimSuffix(name, filepath.Ext(name)) + want
	case want != "" && ext != want && strings.HasPrefix(mt, "video/") && !isMediaExt(ext):
		name += want
	}
	// "a.mp4.mp4" -> "a.mp4"
	if e := filepath.Ext(name); e != "" {
		base := strings.TrimSuffix(name, e)
		if strings.EqualFold(filepath.Ext(base), e) {
			name = base
		}
	}
	return name
}

func isMediaExt(ext string) bool {
	switch ext {
	case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".flv", ".ts", ".3gp", ".mpg", ".mpeg", ".wmv", ".ogv":
		return true
	}
	return false
}

var commonExt = map[string]string{
	"video/mp4":                    ".mp4",
	"video/webm":                   ".webm",
	"video/x-matroska":             ".mkv",
	"video/x-flv":                  ".flv",
	"video/mp2t":                   ".ts",
	"audio/mpeg":                   ".mp3",
	"audio/mp4":                    ".m4a",
	"audio/ogg":                    ".ogg",
	"application/zip":              ".zip",
	"application/pdf":              ".pdf",
	"application/x-rar-compressed": ".rar",
	"application/x-7z-compressed":  ".7z",
	"application/x-msdownload":     ".exe",
	"application/x-bittorrent":     ".torrent",
	"video/quicktime":              ".mov",
	"video/x-msvideo":              ".avi",
	"video/3gpp":                   ".3gp",
	"audio/aac":                    ".aac",
	"audio/x-m4a":                  ".m4a",
	"audio/wav":                    ".wav",
	"audio/x-wav":                  ".wav",
	"audio/flac":                   ".flac",
	"audio/opus":                   ".opus",
	"audio/webm":                   ".weba",
	"application/x-zip-compressed": ".zip",
	"application/gzip":             ".gz",
	"application/x-iso9660-image":  ".iso",
	"application/vnd.android.package-archive": ".apk",
	"application/vnd.apple.mpegurl":           ".m3u8",
	"application/dash+xml":                    ".mpd",
	"image/jpeg":                              ".jpg",
	"image/png":                               ".png",
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
