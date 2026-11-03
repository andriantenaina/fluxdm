package extractors

import (
	"regexp"
	"strconv"
	"strings"
)

// packedRe matches Dean Edwards' packer: eval(function(p,a,c,k,e,d){...}('payload',62,123,'a|b|c'.split('|')...
var packedRe = regexp.MustCompile(`(?s)}\s*\(\s*'((?:\\'|[^'])*)'\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*'((?:\\'|[^'])*)'\.split\('\|'\)`)

var wordRe = regexp.MustCompile(`\b\w+\b`)

const alphabet62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func decodeBase(s string, base int) int {
	if base <= 36 {
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			return -1
		}
		return int(n)
	}
	n := 0
	for _, c := range s {
		i := strings.IndexRune(alphabet62, c)
		if i < 0 || i >= base {
			return -1
		}
		n = n*base + i
	}
	return n
}

// Unpack returns the unpacked source of every packed script found in page.
func Unpack(page string) []string {
	var out []string
	for _, m := range packedRe.FindAllStringSubmatch(page, -1) {
		payload := strings.ReplaceAll(m[1], `\'`, `'`)
		base, _ := strconv.Atoi(m[2])
		count, _ := strconv.Atoi(m[3])
		keys := strings.Split(m[4], "|")
		if base < 2 || base > 62 {
			continue
		}
		res := wordRe.ReplaceAllStringFunc(payload, func(w string) string {
			i := decodeBase(w, base)
			if i >= 0 && i < len(keys) && i < count+1 && keys[i] != "" {
				return keys[i]
			}
			return w
		})
		out = append(out, strings.ReplaceAll(res, `\\`, `\`))
	}
	return out
}
