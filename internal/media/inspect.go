package media

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

// Quality is one choice shown to the user (extension, remote, dialogs).
type Quality struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Height       int    `json:"height,omitempty"`
	Bandwidth    int64  `json:"bandwidth,omitempty"`
	SizeEstimate int64  `json:"sizeEstimate,omitempty"`
	Ext          string `json:"ext"`
}

// Inspection is a user-facing description of a media: real title, qualities,
// estimated sizes and the extension of the final file.
type Inspection struct {
	Title          string    `json:"title"`
	Duration       float64   `json:"duration"`
	Qualities      []Quality `json:"qualities"`
	DefaultQuality string    `json:"defaultQuality"`
	OutputExt      string    `json:"outputExt"`
	FileName       string    `json:"fileName"`
	Live           bool      `json:"live"`
	Thumbnail      string    `json:"thumbnail,omitempty"`
}

// InspectInput is what the caller knows about the media.
type InspectInput struct {
	Kind     string // hls | dash | direct | page | "" (auto)
	URL      string
	Title    string // page title (preferred)
	FileName string // server file name (Content-Disposition)
	Mime     string
	Size     int64
	Headers  map[string]string
}

var mimeExt = map[string]string{
	"video/mp4": "mp4", "video/webm": "webm", "video/x-matroska": "mkv", "video/quicktime": "mov",
	"video/x-flv": "flv", "video/mp2t": "ts", "video/3gpp": "3gp", "video/x-msvideo": "avi",
	"audio/mpeg": "mp3", "audio/mp4": "m4a", "audio/x-m4a": "m4a", "audio/ogg": "ogg", "audio/opus": "opus",
	"audio/aac": "aac", "audio/wav": "wav", "audio/x-wav": "wav", "audio/flac": "flac", "audio/webm": "weba",
}

// ExtForMime returns the extension (without dot) for a media MIME type.
func ExtForMime(m string) string {
	m = strings.ToLower(strings.TrimSpace(strings.SplitN(m, ";", 2)[0]))
	return mimeExt[m]
}

func qualityLabel(h int) string {
	switch {
	case h >= 2160:
		return "4K"
	case h >= 1440:
		return "1440p"
	case h > 0:
		return fmt.Sprintf("%dp", h)
	}
	return "Auto"
}

// FinalName builds "Title (1080p).mp4".
func FinalName(title, label, ext string) string {
	t := CleanTitle(title)
	if t == "" {
		t = "video"
	}
	if label != "" && label != "Auto" && label != "Original" {
		t += " (" + label + ")"
	}
	return engine.SanitizeFileName(t + "." + ext)
}

// Inspect analyzes a media and returns what to display. It never needs
// external tools for HLS/DASH/direct files; pages use yt-dlp.
func (rt *Runtime) Inspect(ctx context.Context, in InspectInput) (*Inspection, error) {
	s := rt.M.Settings()
	streamExt := s.HLSOutput
	if streamExt == "" || streamExt == "mp3" {
		streamExt = "mp4"
	}
	kind := in.Kind
	switch kind {
	case "hls":
		kind = store.KindHLS
	case "dash":
		kind = store.KindDASH
	case "direct":
		kind = store.KindHTTP
	case "page", "":
		kind = rt.M.DetectKind(in.URL)
		if kind == store.KindHTTP && in.Kind == "page" {
			kind = store.KindYtdlp
		}
	}

	out := &Inspection{Title: CleanTitle(in.Title)}

	// Direct files: name and size are known without analysis.
	if kind == store.KindHTTP && in.Kind == "direct" {
		ext := ExtForMime(in.Mime)
		if ext == "" {
			ext = strings.TrimPrefix(strings.ToLower(path.Ext(strings.SplitN(in.URL, "?", 2)[0])), ".")
		}
		if ext == "" {
			ext = "mp4"
		}
		name := ""
		if n := strings.TrimSpace(in.FileName); n != "" && !genericNames[strings.ToLower(strings.TrimSuffix(n, path.Ext(n)))] && !strings.HasPrefix(strings.ToLower(n), "videoplayback") {
			name = engine.FixExtension(engine.SanitizeFileName(n), in.Mime)
		}
		if out.Title == "" && name != "" {
			out.Title = CleanTitle(strings.TrimSuffix(name, path.Ext(name)))
		}
		if name == "" {
			name = FinalName(out.Title, "", ext)
		}
		out.OutputExt = strings.TrimPrefix(path.Ext(name), ".")
		out.FileName = name
		out.Qualities = []Quality{{ID: "original", Label: "Original", SizeEstimate: in.Size, Ext: out.OutputExt}}
		out.DefaultQuality = "original"
		return out, nil
	}

	info, err := rt.Analyze(ctx, kind, in.URL, in.Headers, false)
	if err != nil {
		return nil, err
	}
	return rt.FromInfo(info, in.Title, kind), nil
}

// FromInfo builds the user-facing inspection from an analysis result.
func (rt *Runtime) FromInfo(info *VideoInfo, title, kind string) *Inspection {
	s := rt.M.Settings()
	streamExt := s.HLSOutput
	if streamExt == "" || streamExt == "mp3" {
		streamExt = "mp4"
	}
	out := &Inspection{Title: CleanTitle(title)}
	if out.Title == "" {
		out.Title = CleanTitle(info.Title)
	}
	out.Duration = info.Duration
	out.Live = info.Live
	out.Thumbnail = info.Thumbnail

	switch {
	case len(info.Variants) > 0: // HLS master
		seen := map[int]bool{}
		vs := append([]Variant(nil), info.Variants...)
		sort.Slice(vs, func(i, j int) bool {
			return vs[i].Height > vs[j].Height || (vs[i].Height == vs[j].Height && vs[i].Bandwidth > vs[j].Bandwidth)
		})
		for _, v := range vs {
			if seen[v.Height] {
				continue
			}
			seen[v.Height] = true
			q := Quality{ID: v.URL, Label: qualityLabel(v.Height), Height: v.Height, Bandwidth: v.Bandwidth, Ext: streamExt}
			if info.Duration > 0 && v.Bandwidth > 0 {
				q.SizeEstimate = int64(float64(v.Bandwidth) / 8 * info.Duration)
			}
			out.Qualities = append(out.Qualities, q)
		}
	case len(info.Dash) > 0:
		var audioBW int64
		for _, r := range info.Dash {
			if r.Kind == "audio" && r.Bandwidth > audioBW {
				audioBW = r.Bandwidth
			}
		}
		seen := map[int]bool{}
		reps := append([]DashRep(nil), info.Dash...)
		sort.Slice(reps, func(i, j int) bool { return reps[i].Height > reps[j].Height })
		for _, r := range reps {
			if r.Kind != "video" || seen[r.Height] {
				continue
			}
			seen[r.Height] = true
			q := Quality{ID: "rep:" + r.ID, Label: qualityLabel(r.Height), Height: r.Height, Bandwidth: r.Bandwidth + audioBW, Ext: streamExt}
			if info.Duration > 0 {
				q.SizeEstimate = int64(float64(r.Bandwidth+audioBW) / 8 * info.Duration)
			}
			out.Qualities = append(out.Qualities, q)
		}
	case len(info.Formats) > 0 && info.Source != "direct": // yt-dlp
		best := map[int]int64{}
		var audio int64
		for _, f := range info.Formats {
			if f.VCodec == "none" && f.Filesize > audio {
				audio = f.Filesize
			}
			if f.Height > 0 && f.VCodec != "none" && f.Filesize > best[f.Height] {
				best[f.Height] = f.Filesize
			} else if f.Height > 0 && f.VCodec != "none" {
				if _, ok := best[f.Height]; !ok {
					best[f.Height] = 0
				}
			}
		}
		var hs []int
		for h := range best {
			hs = append(hs, h)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(hs)))
		for _, h := range hs {
			size := best[h]
			if size > 0 {
				size += audio
			}
			out.Qualities = append(out.Qualities, Quality{ID: fmt.Sprintf("h:%d", h), Label: qualityLabel(h), Height: h, SizeEstimate: size, Ext: streamExt})
		}
	}
	if len(out.Qualities) == 0 {
		out.Qualities = []Quality{{ID: "best", Label: "Auto", Ext: streamExt}}
	}
	if kind == store.KindYtdlp || kind == store.KindHoster || len(info.Audio) > 0 {
		out.Qualities = append(out.Qualities, Quality{ID: "audio", Label: "Audio", Ext: "mp3"})
	}
	// default = highest quality within the preferred max height
	def := out.Qualities[0]
	if pref := s.VideoQuality; pref != "" && pref != "best" && pref != "audio" {
		var h int
		fmt.Sscanf(pref, "%d", &h)
		for _, q := range out.Qualities {
			if q.Height > 0 && q.Height <= h {
				def = q
				break
			}
		}
	}
	out.DefaultQuality = def.ID
	out.OutputExt = def.Ext
	out.FileName = FinalName(out.Title, def.Label, def.Ext)
	return out
}
