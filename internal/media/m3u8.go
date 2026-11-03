// Package media downloads video streams: HLS (.m3u8 with .ts / fMP4
// segments, AES-128), DASH (.mpd), pages handled by yt-dlp, and remuxes with ffmpeg.
package media

import (
	"bufio"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Variant is one quality of an HLS master playlist.
type Variant struct {
	URL        string  `json:"url"`
	Bandwidth  int64   `json:"bandwidth"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Codecs     string  `json:"codecs"`
	FrameRate  float64 `json:"frameRate"`
	AudioGroup string  `json:"audioGroup,omitempty"`
	Name       string  `json:"name,omitempty"`
}

// Rendition is an alternative audio/subtitle track (#EXT-X-MEDIA).
type Rendition struct {
	Type     string `json:"type"` // AUDIO | SUBTITLES | VIDEO
	GroupID  string `json:"groupId"`
	Name     string `json:"name"`
	Language string `json:"language"`
	URI      string `json:"uri"`
	Default  bool   `json:"default"`
}

// Key is an #EXT-X-KEY.
type Key struct {
	Method string `json:"method"` // NONE | AES-128 | SAMPLE-AES
	URI    string `json:"uri"`
	IV     []byte `json:"iv,omitempty"`
}

// ByteRange is [offset, length].
type ByteRange struct {
	Offset int64 `json:"o"`
	Length int64 `json:"l"`
}

// SegRef is one downloadable piece (HLS segment, DASH segment or init).
type SegRef struct {
	URL   string     `json:"url"`
	Range *ByteRange `json:"range,omitempty"`
	Key   *Key       `json:"key,omitempty"`
	Seq   int64      `json:"seq"`
	Dur   float64    `json:"dur,omitempty"`
}

// Playlist is a parsed m3u8.
type Playlist struct {
	Master         bool
	Variants       []Variant
	Renditions     []Rendition
	Segments       []SegRef
	Init           *SegRef // #EXT-X-MAP (fMP4)
	TargetDuration float64
	MediaSequence  int64
	Ended          bool
	SampleAES      bool // encrypted with SAMPLE-AES / DRM -> not supported
	Duration       float64
}

// ParseM3U8 parses a master or media playlist. base resolves relative URIs.
func ParseM3U8(body string, base *url.URL) (*Playlist, error) {
	body = strings.TrimPrefix(body, "\uFEFF")
	if !strings.Contains(body, "#EXTM3U") {
		return nil, errors.New("ce n'est pas une playlist HLS")
	}
	p := &Playlist{}
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	var (
		pendingVariant *Variant
		curKey         *Key
		curDur         float64
		curRange       *ByteRange
		nextOffset     int64
		seq            int64
		seqSet         bool
	)
	resolve := func(u string) string {
		u = strings.TrimSpace(u)
		if base == nil {
			return u
		}
		ref, err := url.Parse(u)
		if err != nil {
			return u
		}
		r := base.ResolveReference(ref)
		// keep query params of the playlist (signed CDNs) when the segment has none
		if ref.RawQuery == "" && !ref.IsAbs() && base.RawQuery != "" && strings.Contains(base.RawQuery, "token") {
			r.RawQuery = base.RawQuery
		}
		return r.String()
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			tag, val, _ := strings.Cut(line, ":")
			switch tag {
			case "#EXT-X-STREAM-INF":
				a := parseAttrs(val)
				v := &Variant{Codecs: a["CODECS"], AudioGroup: a["AUDIO"], Name: a["NAME"]}
				v.Bandwidth, _ = strconv.ParseInt(a["BANDWIDTH"], 10, 64)
				if v.Bandwidth == 0 {
					v.Bandwidth, _ = strconv.ParseInt(a["AVERAGE-BANDWIDTH"], 10, 64)
				}
				if res := a["RESOLUTION"]; res != "" {
					if w, h, ok := strings.Cut(strings.ToLower(res), "x"); ok {
						v.Width, _ = strconv.Atoi(w)
						v.Height, _ = strconv.Atoi(h)
					}
				}
				v.FrameRate, _ = strconv.ParseFloat(a["FRAME-RATE"], 64)
				pendingVariant = v
				p.Master = true
			case "#EXT-X-MEDIA":
				a := parseAttrs(val)
				r := Rendition{Type: a["TYPE"], GroupID: a["GROUP-ID"], Name: a["NAME"], Language: a["LANGUAGE"], Default: a["DEFAULT"] == "YES"}
				if a["URI"] != "" {
					r.URI = resolve(a["URI"])
				}
				p.Renditions = append(p.Renditions, r)
				p.Master = true
			case "#EXT-X-TARGETDURATION":
				p.TargetDuration, _ = strconv.ParseFloat(val, 64)
			case "#EXT-X-MEDIA-SEQUENCE":
				seq, _ = strconv.ParseInt(val, 10, 64)
				p.MediaSequence = seq
				seqSet = true
			case "#EXT-X-ENDLIST":
				p.Ended = true
			case "#EXT-X-PLAYLIST-TYPE":
				if strings.EqualFold(val, "VOD") {
					p.Ended = true
				}
			case "#EXTINF":
				d, _, _ := strings.Cut(val, ",")
				curDur, _ = strconv.ParseFloat(strings.TrimSpace(d), 64)
			case "#EXT-X-BYTERANGE":
				curRange = parseByteRange(val, nextOffset)
			case "#EXT-X-KEY":
				a := parseAttrs(val)
				k := &Key{Method: strings.ToUpper(a["METHOD"])}
				if k.Method == "NONE" || k.Method == "" {
					curKey = nil
					continue
				}
				if a["URI"] != "" {
					k.URI = resolve(a["URI"])
				}
				if iv := a["IV"]; iv != "" {
					iv = strings.TrimPrefix(strings.TrimPrefix(iv, "0x"), "0X")
					if b, err := hex.DecodeString(iv); err == nil {
						k.IV = b
					}
				}
				if k.Method != "AES-128" {
					// SAMPLE-AES, SAMPLE-AES-CTR, ISO-23001-7 (FairPlay/Widevine...): DRM, not supported
					p.SampleAES = true
				}
				curKey = k
			case "#EXT-X-MAP":
				a := parseAttrs(val)
				m := &SegRef{URL: resolve(a["URI"])}
				if br := a["BYTERANGE"]; br != "" {
					m.Range = parseByteRange(br, 0)
				}
				if p.Init == nil {
					p.Init = m
				}
			}
			continue
		}
		// URI line
		if pendingVariant != nil {
			pendingVariant.URL = resolve(line)
			p.Variants = append(p.Variants, *pendingVariant)
			pendingVariant = nil
			continue
		}
		if !seqSet {
			seqSet = true
		}
		s := SegRef{URL: resolve(line), Seq: seq, Dur: curDur, Range: curRange}
		if curKey != nil {
			k := *curKey
			s.Key = &k
		}
		if curRange != nil {
			nextOffset = curRange.Offset + curRange.Length
		}
		p.Duration += curDur
		p.Segments = append(p.Segments, s)
		seq++
		curDur, curRange = 0, nil
	}
	if !p.Master && len(p.Segments) == 0 {
		return nil, errors.New("playlist vide")
	}
	return p, sc.Err()
}

func parseByteRange(v string, nextOffset int64) *ByteRange {
	n, o, hasO := strings.Cut(strings.Trim(v, `"`), "@")
	l, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		return nil
	}
	br := &ByteRange{Length: l, Offset: nextOffset}
	if hasO {
		br.Offset, _ = strconv.ParseInt(o, 10, 64)
	}
	return br
}

// parseAttrs parses an HLS attribute list: KEY=VALUE,KEY="quoted, value".
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.ToUpper(strings.TrimSpace(s[:eq]))
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				val, s = s[1:], ""
			} else {
				val = s[1 : end+1]
				s = s[end+2:]
			}
			if i := strings.IndexByte(s, ','); i >= 0 {
				s = s[i+1:]
			} else {
				s = ""
			}
		} else {
			if i := strings.IndexByte(s, ','); i >= 0 {
				val, s = s[:i], s[i+1:]
			} else {
				val, s = s, ""
			}
		}
		out[key] = strings.TrimSpace(val)
	}
	return out
}

// BestVariant picks the highest-bandwidth variant whose height <= maxHeight (0 = no limit).
func BestVariant(vs []Variant, maxHeight int) *Variant {
	var best *Variant
	for i := range vs {
		v := &vs[i]
		if maxHeight > 0 && v.Height > maxHeight {
			continue
		}
		if best == nil || v.Height > best.Height || (v.Height == best.Height && v.Bandwidth > best.Bandwidth) {
			best = v
		}
	}
	if best == nil && len(vs) > 0 {
		// everything is above the limit: take the smallest
		best = &vs[0]
		for i := range vs {
			if vs[i].Height < best.Height {
				best = &vs[i]
			}
		}
	}
	return best
}
