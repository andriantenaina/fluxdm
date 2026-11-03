package media

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"fluxdm/internal/core"
	"fluxdm/internal/store"
)

type mpd struct {
	XMLName  xml.Name    `xml:"MPD"`
	Type     string      `xml:"type,attr"`
	Duration string      `xml:"mediaPresentationDuration,attr"`
	BaseURL  []string    `xml:"BaseURL"`
	Periods  []mpdPeriod `xml:"Period"`
}

type mpdPeriod struct {
	Duration string       `xml:"duration,attr"`
	BaseURL  []string     `xml:"BaseURL"`
	Sets     []mpdAdapSet `xml:"AdaptationSet"`
}

type mpdProtection struct {
	SchemeIDURI string `xml:"schemeIdUri,attr"`
}

type mpdAdapSet struct {
	MimeType    string          `xml:"mimeType,attr"`
	ContentType string          `xml:"contentType,attr"`
	Lang        string          `xml:"lang,attr"`
	BaseURL     []string        `xml:"BaseURL"`
	Template    *mpdTemplate    `xml:"SegmentTemplate"`
	List        *mpdList        `xml:"SegmentList"`
	Protection  []mpdProtection `xml:"ContentProtection"`
	Reps        []mpdRep        `xml:"Representation"`
}

type mpdRep struct {
	ID         string          `xml:"id,attr"`
	Bandwidth  int64           `xml:"bandwidth,attr"`
	Width      int             `xml:"width,attr"`
	Height     int             `xml:"height,attr"`
	Codecs     string          `xml:"codecs,attr"`
	MimeType   string          `xml:"mimeType,attr"`
	BaseURL    []string        `xml:"BaseURL"`
	Template   *mpdTemplate    `xml:"SegmentTemplate"`
	List       *mpdList        `xml:"SegmentList"`
	Base       *mpdSegBase     `xml:"SegmentBase"`
	Protection []mpdProtection `xml:"ContentProtection"`
}

type mpdTemplate struct {
	Media          string       `xml:"media,attr"`
	Initialization string       `xml:"initialization,attr"`
	StartNumber    *int64       `xml:"startNumber,attr"`
	Timescale      int64        `xml:"timescale,attr"`
	Duration       int64        `xml:"duration,attr"`
	Timeline       *mpdTimeline `xml:"SegmentTimeline"`
}

type mpdTimeline struct {
	S []struct {
		T *int64 `xml:"t,attr"`
		D int64  `xml:"d,attr"`
		R int64  `xml:"r,attr"`
	} `xml:"S"`
}

type mpdList struct {
	Init *struct {
		SourceURL string `xml:"sourceURL,attr"`
		Range     string `xml:"range,attr"`
	} `xml:"Initialization"`
	Segs []struct {
		Media      string `xml:"media,attr"`
		MediaRange string `xml:"mediaRange,attr"`
	} `xml:"SegmentURL"`
}

type mpdSegBase struct {
	IndexRange string `xml:"indexRange,attr"`
}

// DashRep describes a selectable representation (for the quality dialog).
type DashRep struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // video | audio | text
	Bandwidth int64  `json:"bandwidth"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Codecs    string `json:"codecs"`
	Lang      string `json:"lang,omitempty"`
}

var isoDurRe = regexp.MustCompile(`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// parseISODuration parses "PT1H2M3.5S" into seconds.
func parseISODuration(s string) float64 {
	m := isoDurRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	f := func(i int) float64 { v, _ := strconv.ParseFloat(m[i], 64); return v }
	return f(1)*86400 + f(2)*3600 + f(3)*60 + f(4)
}

func joinBase(base *url.URL, parts ...[]string) *url.URL {
	u := base
	for _, p := range parts {
		if len(p) > 0 && strings.TrimSpace(p[0]) != "" {
			if r, err := url.Parse(strings.TrimSpace(p[0])); err == nil {
				u = u.ResolveReference(r)
			}
		}
	}
	return u
}

var tmplRe = regexp.MustCompile(`\$(RepresentationID|Number|Time|Bandwidth)(%0(\d+)d)?\$`)

func fillTemplate(t string, repID string, bw int64, number, tm int64) string {
	out := tmplRe.ReplaceAllStringFunc(t, func(m string) string {
		sm := tmplRe.FindStringSubmatch(m)
		var v string
		switch sm[1] {
		case "RepresentationID":
			return repID
		case "Number":
			v = strconv.FormatInt(number, 10)
		case "Time":
			v = strconv.FormatInt(tm, 10)
		case "Bandwidth":
			v = strconv.FormatInt(bw, 10)
		}
		if sm[3] != "" {
			w, _ := strconv.Atoi(sm[3])
			for len(v) < w {
				v = "0" + v
			}
		}
		return v
	})
	return strings.ReplaceAll(out, "$$", "$")
}

func parseRange(r string) *ByteRange {
	a, b, ok := strings.Cut(r, "-")
	if !ok {
		return nil
	}
	s, err1 := strconv.ParseInt(a, 10, 64)
	e, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil || e < s {
		return nil
	}
	return &ByteRange{Offset: s, Length: e - s + 1}
}

func setKind(as mpdAdapSet, r mpdRep) string {
	k := strings.ToLower(as.ContentType + " " + as.MimeType + " " + r.MimeType)
	switch {
	case strings.Contains(k, "video"):
		return "video"
	case strings.Contains(k, "audio"):
		return "audio"
	case strings.Contains(k, "text"), strings.Contains(k, "vtt"), strings.Contains(k, "ttml"):
		return "text"
	}
	if r.Height > 0 {
		return "video"
	}
	return "audio"
}

// repSegments builds the segment list of one representation in a period.
func repSegments(base *url.URL, as mpdAdapSet, r mpdRep, periodDur float64) (*SegRef, []SegRef, error) {
	b := joinBase(base, as.BaseURL, r.BaseURL)
	res := func(s string) string {
		ref, err := url.Parse(s)
		if err != nil {
			return s
		}
		return b.ResolveReference(ref).String()
	}
	tpl := r.Template
	if tpl == nil {
		tpl = as.Template
	}
	list := r.List
	if list == nil {
		list = as.List
	}
	switch {
	case tpl != nil:
		ts := tpl.Timescale
		if ts == 0 {
			ts = 1
		}
		start := int64(1)
		if tpl.StartNumber != nil {
			start = *tpl.StartNumber
		}
		var init *SegRef
		if tpl.Initialization != "" {
			init = &SegRef{URL: res(fillTemplate(tpl.Initialization, r.ID, r.Bandwidth, 0, 0))}
		}
		var segs []SegRef
		if tpl.Timeline != nil {
			var t int64
			n := start
			for _, s := range tpl.Timeline.S {
				if s.T != nil {
					t = *s.T
				}
				rep := s.R
				if rep < 0 { // repeat until end of period
					if periodDur > 0 && s.D > 0 {
						rep = int64(math.Ceil((periodDur*float64(ts)-float64(t))/float64(s.D))) - 1
					} else {
						rep = 0
					}
				}
				for i := int64(0); i <= rep; i++ {
					segs = append(segs, SegRef{URL: res(fillTemplate(tpl.Media, r.ID, r.Bandwidth, n, t)), Seq: n, Dur: float64(s.D) / float64(ts)})
					t += s.D
					n++
				}
			}
		} else if tpl.Duration > 0 {
			if periodDur <= 0 {
				return nil, nil, errors.New("durée du flux DASH inconnue (direct non pris en charge)")
			}
			count := int64(math.Ceil(periodDur * float64(ts) / float64(tpl.Duration)))
			for i := int64(0); i < count; i++ {
				n := start + i
				segs = append(segs, SegRef{URL: res(fillTemplate(tpl.Media, r.ID, r.Bandwidth, n, i*tpl.Duration)), Seq: n})
			}
		}
		return init, segs, nil
	case list != nil:
		var init *SegRef
		if list.Init != nil {
			src := list.Init.SourceURL
			if src == "" {
				src = b.String()
			}
			init = &SegRef{URL: res(src), Range: parseRange(list.Init.Range)}
		}
		var segs []SegRef
		for i, s := range list.Segs {
			src := s.Media
			if src == "" {
				src = b.String()
			}
			segs = append(segs, SegRef{URL: res(src), Range: parseRange(s.MediaRange), Seq: int64(i)})
		}
		return init, segs, nil
	default:
		// SegmentBase / single file: download the whole file as one segment.
		return nil, []SegRef{{URL: b.String()}}, nil
	}
}

// parseMPD returns the manifest plus selectable representations.
func parseMPD(body string) (*mpd, error) {
	var m mpd
	if err := xml.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("manifeste DASH invalide : %w", err)
	}
	if len(m.Periods) == 0 {
		return nil, errors.New("manifeste DASH vide")
	}
	return &m, nil
}

// DashReps lists representations of the first period (quality dialog).
func DashReps(body string) ([]DashRep, error) {
	m, err := parseMPD(body)
	if err != nil {
		return nil, err
	}
	var out []DashRep
	for _, as := range m.Periods[0].Sets {
		for _, r := range as.Reps {
			out = append(out, DashRep{ID: r.ID, Kind: setKind(as, r), Bandwidth: r.Bandwidth, Width: r.Width, Height: r.Height, Codecs: r.Codecs, Lang: as.Lang})
		}
	}
	return out, nil
}

// resolveDASH selects the video/audio representations and builds tracks.
func (rt *Runtime) resolveDASH(ctx context.Context, f *fetcher, it *store.Item) (*streamState, error) {
	body, final, err := f.getText(ctx, it.URL)
	if err != nil {
		return nil, err
	}
	m, err := parseMPD(body)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(m.Type, "dynamic") {
		return nil, errors.New("les directs DASH ne sont pas pris en charge")
	}
	base, _ := url.Parse(final)
	base = joinBase(base, m.BaseURL)
	total := parseISODuration(m.Duration)
	maxH, audioOnly := maxHeightFor(it, rt.M.Settings())
	wantVideo := core.OptString(it.Options, "videoRep")
	wantAudio := core.OptString(it.Options, "audioRep")
	lang := core.OptString(it.Options, "audioLang")

	video := Track{Name: "video", FMP4: true}
	audio := Track{Name: "audio", FMP4: true}
	for pi, p := range m.Periods {
		pdur := parseISODuration(p.Duration)
		if pdur == 0 && len(m.Periods) == 1 {
			pdur = total
		}
		pbase := joinBase(base, p.BaseURL)
		var bestV, bestA *mpdRep
		var setV, setA mpdAdapSet
		for _, as := range p.Sets {
			for ri := range as.Reps {
				r := as.Reps[ri]
				if len(as.Protection) > 0 || len(r.Protection) > 0 {
					if hasDRM(as.Protection) || hasDRM(r.Protection) {
						return nil, ErrDRM
					}
				}
				switch setKind(as, r) {
				case "video":
					if audioOnly {
						continue
					}
					if wantVideo != "" {
						if r.ID == wantVideo {
							bestV, setV = &as.Reps[ri], as
						}
						continue
					}
					if maxH > 0 && r.Height > maxH {
						continue
					}
					if bestV == nil || r.Height > bestV.Height || (r.Height == bestV.Height && r.Bandwidth > bestV.Bandwidth) {
						bestV, setV = &as.Reps[ri], as
					}
				case "audio":
					if wantAudio != "" {
						if r.ID == wantAudio {
							bestA, setA = &as.Reps[ri], as
						}
						continue
					}
					better := bestA == nil || r.Bandwidth > bestA.Bandwidth
					if lang != "" && bestA != nil {
						cur := strings.HasPrefix(setA.Lang, lang)
						cand := strings.HasPrefix(as.Lang, lang)
						if cand != cur {
							better = cand
						}
					}
					if better {
						bestA, setA = &as.Reps[ri], as
					}
				}
			}
		}
		if bestV != nil {
			init, segs, err := repSegments(pbase, setV, *bestV, pdur)
			if err != nil {
				return nil, err
			}
			if pi == 0 {
				video.Init = init
			}
			video.Segs = append(video.Segs, segs...)
		}
		if bestA != nil {
			init, segs, err := repSegments(pbase, setA, *bestA, pdur)
			if err != nil {
				return nil, err
			}
			if pi == 0 {
				audio.Init = init
			}
			audio.Segs = append(audio.Segs, segs...)
		}
	}
	st := &streamState{}
	if len(video.Segs) > 0 {
		st.Tracks = append(st.Tracks, video)
	}
	if len(audio.Segs) > 0 {
		st.Tracks = append(st.Tracks, audio)
	}
	if len(st.Tracks) == 0 {
		return nil, errors.New("aucune piste exploitable dans le manifeste DASH")
	}
	// renumber sequence for state file names
	for ti := range st.Tracks {
		for si := range st.Tracks[ti].Segs {
			st.Tracks[ti].Segs[si].Seq = int64(si)
		}
	}
	return st, nil
}

func hasDRM(ps []mpdProtection) bool {
	for _, p := range ps {
		s := strings.ToLower(p.SchemeIDURI)
		// "urn:mpeg:dash:mp4protection:2011" alone signals CENC; any of them means DRM.
		if s != "" {
			return true
		}
	}
	return false
}

// RunDASH is the runner for store.KindDASH.
func (rt *Runtime) RunDASH(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	f := newFetcher(m, it.Headers)
	var st *streamState
	if len(it.State) > 0 {
		var s streamState
		if json.Unmarshal(it.State, &s) == nil && len(s.Tracks) > 0 {
			st = &s
		}
	}
	if st == nil {
		var err error
		if st, err = rt.resolveDASH(ctx, f, it); err != nil {
			return err
		}
	}
	return rt.runStream(ctx, f, it, rep, st, func(context.Context, *fetcher, *streamState, *streamJob) (bool, error) { return true, nil })
}
