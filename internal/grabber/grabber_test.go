package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"fluxdm/internal/core"
)

func TestCrawl(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><a href="/page2.html">p2</a><a href="https://other.example/x.html">ext</a>
<img src="/img/a.jpg"><a href="/files/doc.pdf">Doc</a><img srcset="/img/b.png 1x, /img/b2.png 2x"></html>`)
	})
	mux.HandleFunc("/page2.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<a href="/files/archive.zip">zip</a><a href="/page3.html">p3</a>`)
	})
	mux.HandleFunc("/page3.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<a href="/files/deep.zip">deep</a>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	m, err := core.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	g := New(m)
	done := make(chan RunStatus, 10)
	g.OnStatus = func(s RunStatus) {
		if !s.Running {
			done <- s
		}
	}
	p, err := g.Save(Project{StartURL: srv.URL + "/", Depth: 1, SameHost: true, Include: []string{"image", "zip", "pdf"}, Dir: filepath.Join(dir, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Start(p.ID); err != nil {
		t.Fatal(err)
	}
	var st RunStatus
	select {
	case st = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("délai dépassé")
	}
	found := map[string]bool{}
	for _, f := range st.Found {
		u, _ := url.Parse(f.URL)
		found[u.Path] = true
	}
	for _, want := range []string{"/img/a.jpg", "/files/doc.pdf", "/img/b.png", "/img/b2.png", "/files/archive.zip"} {
		if !found[want] {
			t.Errorf("%s non trouvé (%v)", want, found)
		}
	}
	if found["/files/deep.zip"] {
		t.Error("profondeur 1 dépassée")
	}
	if st.Pages != 2 {
		t.Errorf("pages = %d, attendu 2", st.Pages)
	}
	// second run: nothing new
	if err := g.Start(p.ID); err != nil {
		t.Fatal(err)
	}
	st = <-done
	for _, f := range st.Found {
		if f.New {
			t.Errorf("%s marqué nouveau au 2e passage", f.URL)
		}
	}
}

func TestMatches(t *testing.T) {
	u, _ := url.Parse("https://x/a/photo_thumb.JPG")
	if !matches([]string{"image"}, u, "jpg") || !matches([]string{"*thumb*"}, u, "jpg") || !matches([]string{"re:photo_"}, u, "jpg") {
		t.Error("filtres non appliqués")
	}
	if matches([]string{"pdf"}, u, "jpg") {
		t.Error("faux positif")
	}
}
