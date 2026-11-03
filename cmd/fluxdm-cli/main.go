// Command fluxdm-cli downloads a single URL with the FluxDM engine (dev/benchmark tool).
//
//	fluxdm-cli [-c 8] [-o dir] [-limit 0] [-H "Header: value"] URL
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"fluxdm/internal/engine"
)

type headerFlags []string

func (h *headerFlags) String() string     { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(v string) error { *h = append(*h, v); return nil }

func main() {
	conns := flag.Int("c", 8, "connexions")
	out := flag.String("o", ".", "dossier de destination")
	limit := flag.Int64("limit", 0, "limite de vitesse en Ko/s (0 = illimitée)")
	h2 := flag.Bool("http2", false, "autoriser HTTP/2")
	var hdrs headerFlags
	flag.Var(&hdrs, "H", "en-tête HTTP (répétable)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage : fluxdm-cli [-c 8] [-o dir] URL")
		os.Exit(2)
	}
	headers := map[string]string{}
	for _, h := range hdrs {
		if k, v, ok := strings.Cut(h, ":"); ok {
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	cfg := engine.DefaultConfig()
	cfg.MaxConns = *conns
	cfg.AllowHTTP2 = *h2
	if *limit > 0 {
		cfg.SpeedLimiter = engine.NewLimiter(*limit * 1024)
	}
	client, err := engine.NewHTTPClient(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	d := engine.NewDownload(client, cfg, engine.Snapshot{URL: flag.Arg(0), Dir: *out, Headers: headers})
	start := time.Now()
	d.OnInfo = func(s engine.Snapshot) {
		fmt.Printf("Fichier : %s\nTaille  : %s  reprise=%v\n", s.Path, human(s.Size), s.Resumable)
	}
	d.OnProgress = func(p engine.Progress) {
		pct := 0.0
		if p.Size > 0 {
			pct = float64(p.Downloaded) * 100 / float64(p.Size)
		}
		fmt.Printf("\r%6.2f%%  %s / %s  %s/s  conn=%d  seg=%d   ", pct, human(p.Downloaded), human(p.Size), human(int64(p.Speed)), p.Conns, len(p.Segments))
	}
	if err := d.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "\nErreur :", err)
		os.Exit(1)
	}
	s := d.Snapshot()
	el := time.Since(start).Seconds()
	fmt.Printf("\nTerminé en %.1fs (%s/s moyen) -> %s\n", el, human(int64(float64(s.Size)/el)), s.Path)
}

func human(n int64) string {
	if n < 0 {
		return "?"
	}
	u := []string{"o", "Ko", "Mo", "Go", "To"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, u[i])
}
