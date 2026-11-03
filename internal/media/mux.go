package media

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
)

// Runtime holds what media runners need.
type Runtime struct {
	M     *core.Manager
	Tools *sidecars.Manager
}

var genericNames = map[string]bool{
	"index": true, "master": true, "playlist": true, "manifest": true, "video": true,
	"chunklist": true, "prog_index": true, "stream": true, "media": true, "v": true, "hls": true,
}

// baseName returns a file name without extension for a stream item.
func baseName(it *store.Item) string {
	if it.FileName != "" {
		n := strings.TrimSuffix(it.FileName, filepath.Ext(it.FileName))
		if n != "" {
			return engine.SanitizeFileName(n)
		}
	}
	if t := strings.TrimSpace(it.Title); t != "" {
		return engine.SanitizeFileName(t)
	}
	if u, err := url.Parse(it.URL); err == nil {
		b := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
		if b != "" && b != "/" && b != "." && !genericNames[strings.ToLower(b)] {
			return engine.SanitizeFileName(b)
		}
		host := strings.TrimPrefix(u.Hostname(), "www.")
		return engine.SanitizeFileName(host + " " + time.Now().Format("2006-01-02 15-04-05"))
	}
	return "video " + time.Now().Format("2006-01-02 15-04-05")
}

// outputFormat returns the wanted container: mp4 | mkv | ts | mp3 | m4a.
func (rt *Runtime) outputFormat(it *store.Item) string {
	if f := core.OptString(it.Options, "output"); f != "" {
		return strings.ToLower(f)
	}
	return rt.M.Settings().HLSOutput
}

// finalize joins/remuxes the downloaded tracks into the final file.
func (rt *Runtime) finalize(ctx context.Context, j *streamJob, it *store.Item, rep core.Reporter) error {
	rep.Status(store.StatusProcessing)
	outFmt := rt.outputFormat(it)
	name := baseName(it)
	if err := os.MkdirAll(it.Dir, 0o755); err != nil {
		return err
	}

	var parts []string
	for ti, t := range j.tracks {
		ext := ".ts"
		if t.FMP4 {
			ext = ".mp4"
		}
		p := filepath.Join(j.tmp, t.Name+"-joined"+ext)
		if err := j.concatTrack(ti, p); err != nil {
			return fmt.Errorf("assemblage des segments : %w", err)
		}
		parts = append(parts, p)
	}

	single := len(parts) == 1
	nativeExt := filepath.Ext(parts[0])
	// No remux needed: raw .ts wanted, or fMP4 already an mp4.
	if single && ((outFmt == "ts" && nativeExt == ".ts") || (outFmt == "mp4" && nativeExt == ".mp4" && rt.Tools.Path(sidecars.FFmpeg) == "")) {
		return rt.place(parts[0], it.Dir, name+nativeExt, it, rep)
	}

	ffmpeg, err := rt.Tools.Ensure(ctx, sidecars.FFmpeg)
	if err != nil {
		if single {
			// Keep the joined stream as is rather than failing.
			return rt.place(parts[0], it.Dir, name+nativeExt, it, rep)
		}
		return fmt.Errorf("ffmpeg est nécessaire pour fusionner l'audio et la vidéo : %w", err)
	}
	ext := "." + outFmt
	if outFmt == "ts" {
		ext = ".ts"
	}
	tmpOut := filepath.Join(j.tmp, "out"+ext)
	if err := Remux(context.WithoutCancel(ctx), ffmpeg, rt.Tools, parts, tmpOut, outFmt); err != nil {
		if outFmt == "mp4" {
			// Codec not allowed in MP4: fall back to MKV.
			tmpOut = filepath.Join(j.tmp, "out.mkv")
			ext = ".mkv"
			if err2 := Remux(context.WithoutCancel(ctx), ffmpeg, rt.Tools, parts, tmpOut, "mkv"); err2 != nil {
				return err
			}
		} else {
			return err
		}
	}
	return rt.place(tmpOut, it.Dir, name+ext, it, rep)
}

// place moves a finished file to its final unique path and reports it.
func (rt *Runtime) place(src, dir, fileName string, it *store.Item, rep core.Reporter) error {
	final := engine.UniquePath(dir, engine.SanitizeFileName(fileName))
	if err := moveFile(src, final); err != nil {
		return err
	}
	var size int64
	if fi, err := os.Stat(final); err == nil {
		size = fi.Size()
	}
	rep.Info(filepath.Base(final), final, size)
	return nil
}

// Remux runs ffmpeg to join inputs (video + audio) into out without re-encoding
// (audio formats mp3/m4a are extracted/encoded).
func Remux(ctx context.Context, ffmpeg string, tools *sidecars.Manager, inputs []string, out, format string) error {
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	switch format {
	case "mp3":
		args = append(args, "-vn", "-c:a", "libmp3lame", "-q:a", "2")
	case "m4a":
		args = append(args, "-vn", "-c:a", "copy")
	default:
		for i := range inputs {
			args = append(args, "-map", fmt.Sprintf("%d:v?", i), "-map", fmt.Sprintf("%d:a?", i))
		}
		args = append(args, "-c", "copy")
		if format == "mp4" {
			args = append(args, "-movflags", "+faststart")
		}
		if format == "ts" {
			args = append(args, "-f", "mpegts")
		}
	}
	args = append(args, out)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Env = tools.Env()
	core.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("ffmpeg : %v %s", err, msg)
	}
	return nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// different volume: copy then delete
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}
