package torrent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"fluxdm/internal/core"
)

func TestInspectTorrentFile(t *testing.T) {
	dir := t.TempDir()
	content := filepath.Join(dir, "album")
	os.MkdirAll(content, 0o755)
	os.WriteFile(filepath.Join(content, "a.txt"), make([]byte, 100000), 0o644)
	os.WriteFile(filepath.Join(content, "b.bin"), make([]byte, 300000), 0o644)

	info := metainfo.Info{PieceLength: 32 << 10}
	if err := info.BuildFromFilePath(content); err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{}
	var err error
	if mi.InfoBytes, err = bencode.Marshal(info); err != nil {
		t.Fatal(err)
	}
	tf := filepath.Join(dir, "album.torrent")
	f, _ := os.Create(tf)
	if err := mi.Write(f); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m, err := core.NewManager(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	s := New(m, m.DataDir)
	got, err := s.Inspect(context.Background(), tf, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "album" || got.Size != 400000 || len(got.Files) != 2 {
		t.Fatalf("inspect = %+v", got)
	}
	mag, err := s.Inspect(context.Background(), "magnet:?xt=urn:btih:"+got.InfoHash+"&dn=album", nil)
	if err != nil || mag.InfoHash != got.InfoHash || mag.Name != "album" {
		t.Fatalf("magnet = %+v %v", mag, err)
	}
}
