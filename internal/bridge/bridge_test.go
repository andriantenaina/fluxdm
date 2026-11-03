package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

type fakeHandler struct{ got []DownloadReq }

func (f *fakeHandler) Download(r DownloadReq) error { f.got = append(f.got, r); return nil }
func (f *fakeHandler) Batch(BatchReq) error         { return nil }
func (f *fakeHandler) Video(VideoReq) error         { return nil }
func (f *fakeHandler) ExtSettings() ExtSettings {
	return ExtSettings{Enabled: true, CaptureExts: []string{"zip"}}
}

func TestBridgeSecurity(t *testing.T) {
	h := &fakeHandler{}
	s, err := Start(t.TempDir(), "test", h)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := "http://127.0.0.1:" + strconv.Itoa(s.Port())

	// a web page cannot obtain the token
	req, _ := http.NewRequest("GET", base+"/v1/hello", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("hello depuis un site web : %d", resp.StatusCode)
	}
	resp.Body.Close()

	// the extension can
	req, _ = http.NewRequest("GET", base+"/v1/hello", nil)
	req.Header.Set("Origin", "chrome-extension://lnbmofjmdcfihhamjdgjdhbclmhpfblk")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var hello struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&hello)
	resp.Body.Close()
	if hello.Token == "" {
		t.Fatal("jeton non renvoyé à l'extension")
	}

	// no token -> 401
	body, _ := json.Marshal(DownloadReq{URL: "https://x/a.zip"})
	resp, _ = http.Post(base+"/v1/download", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sans jeton : %d", resp.StatusCode)
	}
	resp.Body.Close()

	// with token -> handled
	req, _ = http.NewRequest("POST", base+"/v1/download", bytes.NewReader(body))
	req.Header.Set("X-FluxDM-Token", hello.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(h.got) != 1 || h.got[0].URL != "https://x/a.zip" {
		t.Fatalf("download : %d %+v", resp.StatusCode, h.got)
	}

	// discovery file for the native host
	d, err := ReadDiscovery(filepathDir(s.file))
	if err != nil || d.Token != hello.Token || !Alive(d) {
		t.Fatalf("découverte : %+v %v", d, err)
	}
}

func filepathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}

func TestNativeManifest(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(NativeManifest(`C:\FluxDM\fluxdm-host.exe`), &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != HostName || m["type"] != "stdio" {
		t.Fatalf("manifeste : %v", m)
	}
	o := m["allowed_origins"].([]any)
	if len(o) == 0 || o[0] != "chrome-extension://lnbmofjmdcfihhamjdgjdhbclmhpfblk/" {
		t.Fatalf("origines : %v", o)
	}
}
