package sidecars

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("ffmpeg-master/bin/ffmpeg.exe")
	w.Write([]byte("binary"))
	w2, _ := zw.Create("ffmpeg-master/doc/readme.txt")
	w2.Write([]byte("doc"))
	zw.Close()
	f.Close()
	out := filepath.Join(dir, "tools")
	os.MkdirAll(out, 0o755)
	if err := extractZip(zp, out, []string{"ffmpeg.exe"}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(out, "ffmpeg.exe")); err != nil || string(b) != "binary" {
		t.Fatalf("extraction : %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "readme.txt")); err == nil {
		t.Error("fichier non demandé extrait")
	}
	if err := extractZip(zp, out, []string{"absent"}); err == nil {
		t.Error("erreur attendue pour un fichier absent")
	}
}

func TestSources(t *testing.T) {
	for _, tool := range []string{YtDlp, FFmpeg, Deno} {
		s, err := sourceFor(tool)
		if runtime.GOOS == "linux" && tool == FFmpeg {
			continue
		}
		if err != nil || s.url == "" {
			t.Errorf("%s : %v", tool, err)
		}
	}
	m := &Manager{Dir: t.TempDir()}
	env := m.Env()
	found := false
	for _, e := range env {
		if len(e) > 5 && (e[:5] == "PATH=" || e[:5] == "Path=") && len(e) > len(m.Dir) && e[5:5+len(m.Dir)] == m.Dir {
			found = true
		}
	}
	if !found {
		t.Error("le dossier des outils doit être en tête du PATH")
	}
}
