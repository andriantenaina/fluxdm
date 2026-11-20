package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

type fakeHandler struct {
	got      []DownloadReq
	settings ExtSettings
	shown    []string
	controls []ControlReq
}

func (f *fakeHandler) Download(r DownloadReq) (string, error) {
	f.got = append(f.got, r)
	return "id1", nil
}
func (f *fakeHandler) Batch(BatchReq) ([]string, error) { return []string{"a", "b"}, nil }
func (f *fakeHandler) Video(VideoReq) (string, error)   { return "", nil }
func (f *fakeHandler) Inspect(InspectReq) InspectResp {
	return InspectResp{Title: "t"}
}
func (f *fakeHandler) ExtSettings() ExtSettings {
	if f.settings.CaptureExts == nil {
		f.settings = ExtSettings{Enabled: true, CaptureEnabled: true, CaptureExts: []string{"zip"}, VideoButton: true}
	}
	return f.settings
}
func (f *fakeHandler) SaveExtSettings(p ExtSettingsPatch) (ExtSettings, error) {
	s := f.ExtSettings()
	if p.CaptureMinSize != nil {
		s.CaptureMinSize = *p.CaptureMinSize
	}
	if p.VideoButton != nil {
		s.VideoButton = *p.VideoButton
	}
	if p.VideoSkipSites != nil {
		s.VideoSkipSites = *p.VideoSkipSites
	}
	f.settings = s
	return s, nil
}
func (f *fakeHandler) Active() ActiveResp {
	return ActiveResp{Items: []ActiveItem{{ID: "x", FileName: "a.zip", Status: "downloading", Size: 10, Downloaded: 5, Speed: 1, ETA: 5, Kind: "http"}}, Speed: 1, Active: 1}
}
func (f *fakeHandler) Show(id string) error { f.shown = append(f.shown, id); return nil }
func (f *fakeHandler) Control(id, action string) error {
	f.controls = append(f.controls, ControlReq{ID: id, Action: action})
	return nil
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
	var dl struct {
		OK bool   `json:"ok"`
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&dl)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(h.got) != 1 || h.got[0].URL != "https://x/a.zip" {
		t.Fatalf("download : %d %+v", resp.StatusCode, h.got)
	}
	if !dl.OK || dl.ID != "id1" {
		t.Errorf("réponse download sans id : %+v", dl)
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

func TestBridgeExtensionEndpoints(t *testing.T) {
	h := &fakeHandler{}
	s, err := Start(t.TempDir(), "test", h)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := "http://127.0.0.1:" + strconv.Itoa(s.Port())
	do := func(method, path, body string, out any) int {
		t.Helper()
		var rd *bytes.Reader
		if body != "" {
			rd = bytes.NewReader([]byte(body))
		} else {
			rd = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, base+path, rd)
		req.Header.Set("X-FluxDM-Token", s.token)
		req.Header.Set("Origin", "chrome-extension://lnbmofjmdcfihhamjdgjdhbclmhpfblk")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.Header.Get("Access-Control-Allow-Origin") == "" {
			t.Errorf("%s : en-tête CORS absent", path)
		}
		if out != nil {
			json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode
	}

	var act ActiveResp
	if c := do("GET", "/v1/active", "", &act); c != 200 || len(act.Items) != 1 || act.Items[0].ETA != 5 || act.Active != 1 {
		t.Errorf("active : %d %+v", c, act)
	}

	var es ExtSettings
	if c := do("POST", "/v1/settings", `{"captureMinSize":1048576,"videoSkipSites":["youtube.com"]}`, &es); c != 200 {
		t.Fatalf("settings : %d", c)
	}
	if es.CaptureMinSize != 1<<20 || len(es.VideoSkipSites) != 1 || !es.VideoButton || !es.CaptureEnabled {
		t.Errorf("settings partiels mal appliqués : %+v", es)
	}
	if c := do("GET", "/v1/settings", "", &es); c != 200 || es.CaptureMinSize != 1<<20 {
		t.Errorf("GET settings : %d %+v", c, es)
	}

	if c := do("POST", "/v1/show", `{"id":"x"}`, nil); c != 200 || len(h.shown) != 1 || h.shown[0] != "x" {
		t.Errorf("show : %d %v", c, h.shown)
	}
	if c := do("POST", "/v1/show", "", nil); c != 200 || len(h.shown) != 2 || h.shown[1] != "" {
		t.Errorf("show sans corps : %d %v", c, h.shown)
	}

	if c := do("POST", "/v1/control", `{"id":"x","action":"pause"}`, nil); c != 200 || len(h.controls) != 1 {
		t.Errorf("control : %d %v", c, h.controls)
	}
	if c := do("POST", "/v1/control", `{"id":"x","action":"explode"}`, nil); c != 400 {
		t.Errorf("action inconnue acceptée : %d", c)
	}

	var batch struct {
		IDs []string `json:"ids"`
	}
	if c := do("POST", "/v1/batch", `{"pageUrl":"https://p","links":[]}`, &batch); c != 200 || len(batch.IDs) != 2 {
		t.Errorf("batch : %d %+v", c, batch)
	}
	var vid map[string]any
	if c := do("POST", "/v1/video", `{"kind":"hls","url":"https://v/m.m3u8"}`, &vid); c != 200 || vid["id"] != nil {
		t.Errorf("video (dialogue) : %d %+v", c, vid)
	}

	// a web page cannot call the new endpoints
	req, _ := http.NewRequest("GET", base+"/v1/active", nil)
	req.Header.Set("X-FluxDM-Token", s.token)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("origine web : %d", resp.StatusCode)
	}
}
