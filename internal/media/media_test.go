package media

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fluxdm/internal/core"
	"fluxdm/internal/sidecars"
	"fluxdm/internal/store"
)

func TestParseMaster(t *testing.T) {
	body := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Français",LANGUAGE="fr",DEFAULT=YES,URI="audio/fr.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360,CODECS="avc1.4d401e,mp4a.40.2",AUDIO="aud"
low/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2",AUDIO="aud"
https://cdn.example.com/hi/index.m3u8
`
	base, _ := url.Parse("https://site.example.com/v/master.m3u8?token=abc")
	p, err := ParseM3U8(body, base)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Master || len(p.Variants) != 2 || len(p.Renditions) != 1 {
		t.Fatalf("master mal analysé : %+v", p)
	}
	if p.Variants[0].URL != "https://site.example.com/v/low/index.m3u8?token=abc" {
		t.Errorf("URL relative : %s", p.Variants[0].URL)
	}
	if p.Renditions[0].URI != "https://site.example.com/v/audio/fr.m3u8?token=abc" || p.Renditions[0].Language != "fr" {
		t.Errorf("rendition : %+v", p.Renditions[0])
	}
	if v := BestVariant(p.Variants, 720); v == nil || v.Height != 360 {
		t.Errorf("BestVariant(720) = %+v", v)
	}
	if v := BestVariant(p.Variants, 0); v == nil || v.Height != 1080 {
		t.Errorf("BestVariant(0) = %+v", v)
	}
}

func TestParseMedia(t *testing.T) {
	body := `#EXTM3U
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:10
#EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x000102030405060708090a0b0c0d0e0f
#EXTINF:6.0,
seg10.ts
#EXT-X-KEY:METHOD=NONE
#EXTINF:5.5,
#EXT-X-BYTERANGE:1000@0
all.ts
#EXTINF:5.5,
#EXT-X-BYTERANGE:500
all.ts
#EXT-X-ENDLIST
`
	base, _ := url.Parse("https://h.example/p/media.m3u8")
	p, err := ParseM3U8(body, base)
	if err != nil {
		t.Fatal(err)
	}
	if p.Master || !p.Ended || len(p.Segments) != 3 {
		t.Fatalf("media : %+v", p)
	}
	s0 := p.Segments[0]
	if s0.Seq != 10 || s0.Key == nil || s0.Key.URI != "https://h.example/p/key.bin" || len(s0.Key.IV) != 16 {
		t.Errorf("segment 0 : %+v key=%+v", s0, s0.Key)
	}
	if p.Segments[1].Key != nil {
		t.Error("METHOD=NONE doit retirer la clé")
	}
	if r := p.Segments[2].Range; r == nil || r.Offset != 1000 || r.Length != 500 {
		t.Errorf("byterange implicite : %+v", r)
	}
}

func TestSampleAESIsDRM(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://x\",KEYFORMAT=\"com.apple.streamingkeydelivery\"\n#EXTINF:4,\na.ts\n#EXT-X-ENDLIST\n"
	p, err := ParseM3U8(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.SampleAES {
		t.Error("SAMPLE-AES doit être détecté comme DRM")
	}
}

func tsSegment(n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		pkt := make([]byte, 188)
		rand.Read(pkt)
		pkt[0] = 0x47
		b.Write(pkt)
	}
	return b.Bytes()
}

func encrypt(t *testing.T, plain, key, iv []byte) []byte {
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	p := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	blk, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(p))
	cipher.NewCBCEncrypter(blk, iv).CryptBlocks(out, p)
	return out
}

func newTestManager(t *testing.T) (*core.Manager, *Runtime) {
	t.Helper()
	dir := t.TempDir()
	m, err := core.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	s := m.Settings()
	s.DownloadDir = filepath.Join(dir, "dl")
	s.ShowDoneDialog = false
	if err := m.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	tools := sidecars.New(filepath.Join(dir, "tools"), m.HTTPClient, m.EngineConfig)
	rt := New(m, tools)
	rt.Register(nil)
	return m, rt
}

func waitDone(t *testing.T, m *core.Manager, id string) *store.Item {
	return waitFor(t, m, id, 30*time.Second)
}

func waitDoneLong(t *testing.T, m *core.Manager, id string) *store.Item {
	return waitFor(t, m, id, 8*time.Minute)
}

func waitFor(t *testing.T, m *core.Manager, id string, d time.Duration) *store.Item {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		it, _ := m.Get(id)
		if it != nil && (it.Status == store.StatusCompleted || it.Status == store.StatusError) {
			return it
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("délai dépassé")
	return nil
}

func TestHLSEndToEnd(t *testing.T) {
	key := make([]byte, 16)
	rand.Read(key)
	var plain [][]byte
	for i := 0; i < 6; i++ {
		plain = append(plain, tsSegment(50+i))
	}
	png := []byte("\x89PNG\r\n\x1a\n fake image header ")
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://player.example/embed/1" {
			http.Error(w, "referer requis", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=500000,RESOLUTION=640x360\nlow.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720\nhi.m3u8\n")
	})
	mux.HandleFunc("/low.m3u8", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "wrong variant", 500) })
	mux.HandleFunc("/hi.m3u8", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString("#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n")
		for i := range plain {
			if i == 4 {
				b.WriteString("#EXT-X-KEY:METHOD=NONE\n")
			}
			fmt.Fprintf(&b, "#EXTINF:4.0,\nseg%d.png\n", i)
		}
		b.WriteString("#EXT-X-ENDLIST\n")
		fmt.Fprint(w, b.String())
	})
	mux.HandleFunc("/key.bin", func(w http.ResponseWriter, r *http.Request) { w.Write(key) })
	for i := range plain {
		i := i
		mux.HandleFunc(fmt.Sprintf("/seg%d.png", i), func(w http.ResponseWriter, r *http.Request) {
			data := plain[i]
			if i == 5 {
				data = append(append([]byte{}, png...), data...) // disguised segment
			}
			if i < 4 {
				data = encrypt(t, data, key, seqIV(int64(i)))
			}
			w.Write(data)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m, _ := newTestManager(t)
	it, err := m.Add(core.AddRequest{URL: srv.URL + "/master.m3u8", Kind: store.KindHLS, Title: "Mon film",
		Headers: map[string]string{"Referer": "https://player.example/embed/1"}, Options: map[string]any{"output": "ts"}, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	got := waitDone(t, m, it.ID)
	if got.Status != store.StatusCompleted {
		t.Fatalf("échec : %s", got.Error)
	}
	if filepath.Base(got.Path) != "Mon film.ts" {
		t.Errorf("nom final = %s", got.Path)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, bytes.Join(plain, nil)) {
		t.Fatalf("sortie incorrecte (%d octets, attendu %d)", len(data), len(bytes.Join(plain, nil)))
	}
	if _, err := os.Stat(m.TempDir(it.ID)); err == nil {
		t.Error("dossier temporaire non nettoyé")
	}
}

func TestDASHTemplate(t *testing.T) {
	body := `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT10S">
  <BaseURL>https://cdn.example/v/</BaseURL>
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate media="$RepresentationID$/seg-$Number%05d$.m4s" initialization="$RepresentationID$/init.mp4" startNumber="1" timescale="1000" duration="4000"/>
      <Representation id="v720" bandwidth="2000000" width="1280" height="720"/>
      <Representation id="v1080" bandwidth="5000000" width="1920" height="1080"/>
    </AdaptationSet>
    <AdaptationSet mimeType="audio/mp4" lang="fr">
      <SegmentTemplate media="a/$Time$.m4s" initialization="a/init.mp4" timescale="48000">
        <SegmentTimeline><S t="0" d="96000" r="2"/><S d="48000"/></SegmentTimeline>
      </SegmentTemplate>
      <Representation id="aac" bandwidth="128000"/>
    </AdaptationSet>
  </Period>
</MPD>`
	m, err := parseMPD(body)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://site.example/manifest.mpd")
	base = joinBase(base, m.BaseURL)
	p := m.Periods[0]
	init, segs, err := repSegments(base, p.Sets[0], p.Sets[0].Reps[1], 10)
	if err != nil {
		t.Fatal(err)
	}
	if init.URL != "https://cdn.example/v/v1080/init.mp4" || len(segs) != 3 || segs[2].URL != "https://cdn.example/v/v1080/seg-00003.m4s" {
		t.Fatalf("template : init=%v segs=%v", init, segs)
	}
	_, asegs, _ := repSegments(base, p.Sets[1], p.Sets[1].Reps[0], 10)
	if len(asegs) != 4 || asegs[3].URL != "https://cdn.example/v/a/288000.m4s" {
		t.Fatalf("timeline : %v", asegs)
	}
	if d := parseISODuration("PT1H2M3.5S"); d != 3723.5 {
		t.Errorf("durée ISO = %v", d)
	}
}

func TestFixTS(t *testing.T) {
	seg := tsSegment(3)
	if !bytes.Equal(fixTS(append([]byte("GIF89a....garbage"), seg...)), seg) {
		t.Error("préfixe non retiré")
	}
	if !bytes.Equal(fixTS(seg), seg) {
		t.Error("segment valide modifié")
	}
}

func TestYtdlpHosts(t *testing.T) {
	for _, u := range []string{"https://www.youtube.com/watch?v=x", "https://youtu.be/x", "https://m.facebook.com/watch/1", "https://vimeo.com/1"} {
		if !IsYtdlpHost(u) {
			t.Errorf("%s devrait aller à yt-dlp", u)
		}
	}
	if IsYtdlpHost("https://example.com/video.mp4") {
		t.Error("faux positif")
	}
	if f, audio := formatSelector(&store.Item{Options: map[string]any{"maxHeight": float64(720)}}, core.DefaultSettings()); audio || !strings.Contains(f, "height<=720") {
		t.Errorf("sélecteur = %s", f)
	}
}
