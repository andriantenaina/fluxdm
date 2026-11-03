package extractors

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnpack(t *testing.T) {
	page := `<script>eval(function(p,a,c,k,e,d){while(c--)if(k[c])p=p.replace(new RegExp('\\b'+c.toString(a)+'\\b','g'),k[c]);return p}('0 1=\'2\';3({4:"5://6.7/8.9"})',10,10,'var|x|hello|setup|file|https|cdn|example|master|m3u8'.split('|'),0,{}))</script>`
	out := Unpack(page)
	if len(out) != 1 {
		t.Fatalf("Unpack = %v", out)
	}
	if !strings.Contains(out[0], `var x='hello'`) || !strings.Contains(out[0], `file:"https://cdn.example/master.m3u8"`) {
		t.Fatalf("résultat : %s", out[0])
	}
}

// voeEncode is the inverse of voeDecode (test helper).
func voeEncode(jsonStr string) string {
	s2 := base64.StdEncoding.EncodeToString([]byte(jsonStr))
	s1 := shift(reverse(s2), 3)
	t := base64.StdEncoding.EncodeToString([]byte(s1))
	t = t[:5] + "@$" + t[5:10] + "^^" + t[10:]
	return rot13(t)
}

func TestVoeDecode(t *testing.T) {
	enc := voeEncode(`{"source":"https://delivery.example/engine/hls2/01/master.m3u8?t=xyz","direct_access_url":"https://x/y.mp4"}`)
	got, err := voeDecode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://delivery.example/engine/hls2/01/master.m3u8?t=xyz" {
		t.Fatalf("voe = %s", got)
	}
}

func TestBuiltinRulesCompileAndMatch(t *testing.T) {
	e := New(t.TempDir(), func() *http.Client { return http.DefaultClient }, func() string { return "UA" })
	cases := map[string]string{
		"https://voe.sx/e/abc123":                    "Voe",
		"https://dood.li/e/xyz":                      "Doodstream",
		"https://d0000d.com/e/xyz":                   "Doodstream",
		"https://uqload.cx/embed-abcdefgh1234.html":  "Uqload",
		"https://vidmoly.to/embed-abcdefgh1234.html": "Vidmoly",
		"https://filemoon.sx/e/abc":                  "Filemoon",
		"https://streamtape.com/e/abc":               "Streamtape",
		"https://mixdrop.ag/e/abc":                   "Mixdrop",
	}
	for u, want := range cases {
		h := e.Match(u)
		if h == nil || h.Name != want && !strings.HasPrefix(h.Name, want) {
			t.Errorf("Match(%s) = %v, attendu %s", u, h, want)
		}
	}
	if e.Match("https://www.google.com/") != nil {
		t.Error("faux positif sur google.com")
	}
}

func TestDoodstreamRecipe(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/e/abc", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><title>Ma video - DoodStream</title><script>
$.get('/pass_md5/123-45-678/tok9876', function(data){ dsplayer.src = makePlay() });
function makePlay(){ return a + "?token=tok9876&expiry=" + Date.now(); }</script></html>`)
	})
	mux.HandleFunc("/pass_md5/123-45-678/tok9876", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.Header.Get("Referer"), "/e/abc") {
			http.Error(w, "no referer", 403)
			return
		}
		fmt.Fprint(w, srvURL+"/cdn/video~")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	e := New(t.TempDir(), func() *http.Client { return srv.Client() }, func() string { return "UA" })
	// reuse the real Doodstream recipe, matched on the test host
	for _, h := range e.rules.Hosters {
		if h.Name == "Doodstream" {
			h.Hosts = []string{"127\\.0\\.0\\.1"}
			rules, _ := parseRules([]byte(`{"version":1,"hosters":[]}`))
			rules.Hosters = append(rules.Hosters, h)
			h.re = nil
			raw := fmt.Sprintf(`{"version":1,"hosters":[{"name":"Doodstream","hosts":["127\\.0\\.0\\.1"],"steps":%s,"headers":{"Referer":"{origin}/"}}]}`, mustJSON(h.Steps))
			r2, err := parseRules([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			e.rules = r2
		}
	}
	res, err := e.Resolve(context.Background(), srv.URL+"/d/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URL, srv.URL+"/cdn/video~") || !strings.Contains(res.URL, "?token=tok9876&expiry=") {
		t.Fatalf("URL = %s", res.URL)
	}
	if res.Title != "Ma video" || res.Headers["Referer"] != srv.URL+"/" {
		t.Fatalf("résultat : %+v", res)
	}
}

func TestGenericExtractor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><title>Test</title><script>jwplayer("p").setup({sources:[{file:"https:\/\/cdn.example\/hls\/master.m3u8?e=1"}]})</script></html>`)
	}))
	defer srv.Close()
	e := New(t.TempDir(), func() *http.Client { return srv.Client() }, func() string { return "UA" })
	res, err := e.Generic(context.Background(), srv.URL+"/embed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != "https://cdn.example/hls/master.m3u8?e=1" || res.Kind != "hls" {
		t.Fatalf("generic : %+v", res)
	}
}

func TestStreamtape(t *testing.T) {
	raw := `document.getElementById('robotlink').innerHTML = '//streamtape.com/get_video?id=abc&expires=1&ip=x&token=' + ('xcdTOKEN123').substring(1).substring(2);`
	got, err := streamtapeURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://streamtape.com/get_video?id=abc&expires=1&ip=x&token=TOKEN123&stream=1" {
		t.Fatalf("streamtape = %s", got)
	}
}

func mustJSON(v any) string {
	b, err := jsonMarshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
