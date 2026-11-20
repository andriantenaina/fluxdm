package media

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	titleSuffixRe = regexp.MustCompile(`(?i)\s*[-|–—•·:]\s*(youtube|vimeo|dailymotion|facebook|instagram|tiktok|twitter|x|twitch|reddit|voe|doodstream|dood|uqload|vidmoly|filemoon|streamwish|streamtape|mixdrop|netu|watch\s+online|streaming|regarder.*|en streaming.*|vf|vostfr)\s*$`)
	titlePrefixRe = regexp.MustCompile(`(?i)^\s*(watch|regarder|ver|play|lecture)\s+`)
	titleExtRe    = regexp.MustCompile(`(?i)\.(mp4|mkv|webm|avi|mov|m3u8|ts|flv)$`)
	spacesRe      = regexp.MustCompile(`\s+`)
	notifRe       = regexp.MustCompile(`^\(\d+\)\s*`) // "(3) Title" unread counters
)

// CleanTitle turns a page/player title into a good file name base.
func CleanTitle(t string) string {
	t = strings.TrimSpace(t)
	t = notifRe.ReplaceAllString(t, "")
	for i := 0; i < 3; i++ { // "Film - Voe - Watch online"
		n := titleSuffixRe.ReplaceAllString(t, "")
		if n == t {
			break
		}
		t = n
	}
	t = titlePrefixRe.ReplaceAllString(t, "")
	t = titleExtRe.ReplaceAllString(t, "")
	t = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u200b' || r == '\ufeff' {
			return -1
		}
		return r
	}, t)
	t = strings.ReplaceAll(t, "_", " ")
	t = strings.TrimSpace(spacesRe.ReplaceAllString(t, " "))
	switch strings.ToLower(t) {
	case "", "video", "embed", "player", "untitled", "watch", "index", "master", "playlist":
		return ""
	}
	if r := []rune(t); len(r) > 120 {
		t = strings.TrimSpace(string(r[:120]))
	}
	return t
}
