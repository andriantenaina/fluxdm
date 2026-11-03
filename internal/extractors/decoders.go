package extractors

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// decoders are named transformations usable from rules ({"op":"decode","fn":"..."}).
var decoders = map[string]func(string) (string, error){
	"base64":    b64,
	"rot13":     func(s string) (string, error) { return rot13(s), nil },
	"reverse":   func(s string) (string, error) { return reverse(s), nil },
	"urldecode": url.QueryUnescape,
	"unescape":  func(s string) (string, error) { return jsUnescape(s), nil },
	"voe":       voeDecode,
	"trim":      func(s string) (string, error) { return strings.TrimSpace(s), nil },
	"https":     func(s string) (string, error) { return ensureScheme(s), nil },
}

var jsonMarshal = json.Marshal

func b64(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", errors.New("base64 invalide")
}

func rot13(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		}
	}
	return string(b)
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func shift(s string, n int) string {
	r := []rune(s)
	for i := range r {
		r[i] += rune(n)
	}
	return string(r)
}

func jsUnescape(s string) string {
	s = strings.ReplaceAll(s, `\/`, `/`)
	s = strings.ReplaceAll(s, `&`, `&`)
	s = strings.ReplaceAll(s, `=`, `=`)
	return s
}

func ensureScheme(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

// voeDecode decodes the obfuscated JSON blob of Voe pages:
// rot13 -> remove junk markers -> base64 -> shift chars by -3 -> reverse -> base64 -> JSON.
// Returns the best media URL ("source" HLS, else "direct_access_url").
func voeDecode(enc string) (string, error) {
	t := rot13(enc)
	for _, junk := range []string{"@$", "^^", "~@", "%?", "*~", "!!", "#&"} {
		t = strings.ReplaceAll(t, junk, "")
	}
	t = strings.ReplaceAll(t, "_", "")
	s1, err := b64(t)
	if err != nil {
		return "", err
	}
	s2 := reverse(shift(s1, -3))
	s3, err := b64(s2)
	if err != nil {
		return "", err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(s3), &obj); err != nil {
		return "", err
	}
	for _, k := range []string{"source", "hls", "direct_access_url", "file"} {
		if v, ok := obj[k].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", errors.New("aucune source dans les données Voe")
}
