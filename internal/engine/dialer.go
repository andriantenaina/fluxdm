package engine

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// fastDialer adds a DNS cache, spreads connections over every IP address of
// a host (CDNs often throttle per edge server) and races the addresses
// ("Happy Eyeballs") so that a dead IP does not cost a full connect timeout.
// TLS server names are unaffected: http.Transport uses the URL host for SNI.
// Socket buffers are left to the OS: Windows auto-tunes the receive window,
// and a fixed SO_RCVBUF disables that auto-tuning.
type fastDialer struct {
	d      *net.Dialer
	spread bool

	mu    sync.Mutex
	cache map[string]dnsEntry
}

type dnsEntry struct {
	ips []net.IP
	at  time.Time
	seq *atomic.Uint32
}

const (
	dnsTTL       = 60 * time.Second
	dnsKeepStale = 10 * time.Minute // stale entries are a fallback when DNS fails
	raceDelay    = 250 * time.Millisecond
	perIPTimeout = 5 * time.Second
	maxRaceIPs   = 4
)

func (f *fastDialer) lookup(ctx context.Context, host string) ([]net.IP, *atomic.Uint32, error) {
	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]dnsEntry{}
	}
	e, ok := f.cache[host]
	f.mu.Unlock()
	if ok && time.Since(e.at) < dnsTTL {
		return e.ips, e.seq, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		if ok { // stale entry is better than nothing
			return e.ips, e.seq, nil
		}
		return nil, nil, err
	}
	var v4, v6 []net.IP
	for _, a := range addrs {
		if a.IP.To4() != nil {
			v4 = append(v4, a.IP)
		} else {
			v6 = append(v6, a.IP)
		}
	}
	ips := append(v4, v6...) // prefer IPv4 (more reliable on home networks)
	seq := &atomic.Uint32{}
	if ok {
		seq = e.seq
	}
	now := time.Now()
	f.mu.Lock()
	for h, old := range f.cache { // prune expired entries
		if now.Sub(old.at) > dnsKeepStale {
			delete(f.cache, h)
		}
	}
	f.cache[host] = dnsEntry{ips: ips, at: now, seq: seq}
	f.mu.Unlock()
	return ips, seq, nil
}

func tune(c net.Conn) net.Conn {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return c
}

// DialContext dials host:port, rotating over the host's IPs.
func (f *fastDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) != nil || host == "localhost" {
		c, err := f.d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return tune(c), nil
	}
	ips, seq, err := f.lookup(ctx, host)
	ips = filterFamily(ips, network)
	if err != nil || len(ips) == 0 {
		c, err2 := f.d.DialContext(ctx, network, addr)
		if err2 != nil {
			return nil, err2
		}
		return tune(c), nil
	}
	start := 0
	if f.spread {
		start = int(seq.Add(1)-1) % len(ips)
	}
	n := min(len(ips), maxRaceIPs)
	order := make([]string, n)
	for i := range order {
		order[i] = net.JoinHostPort(ips[(start+i)%len(ips)].String(), port)
	}
	return f.race(ctx, network, order)
}

func filterFamily(ips []net.IP, network string) []net.IP {
	if network != "tcp4" && network != "tcp6" {
		return ips
	}
	var out []net.IP
	for _, ip := range ips {
		if (ip.To4() != nil) == (network == "tcp4") {
			out = append(out, ip)
		}
	}
	return out
}

// race starts a connection to the first address, then one more every 250 ms
// (or as soon as one fails); the first established connection wins and the
// others are cancelled or closed.
func (f *fastDialer) race(ctx context.Context, network string, addrs []string) (net.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, len(addrs))
	launched, pending := 0, 0
	launch := func() {
		a := addrs[launched]
		last := launched == len(addrs)-1
		launched++
		pending++
		go func() {
			dctx := ctx
			if !last { // the last candidate keeps the dialer's full timeout
				var c context.CancelFunc
				dctx, c = context.WithTimeout(ctx, perIPTimeout)
				defer c()
			}
			c, err := f.d.DialContext(dctx, network, a)
			ch <- result{c, err}
		}()
	}
	discard := func(n int) {
		go func() {
			for i := 0; i < n; i++ {
				if r := <-ch; r.c != nil {
					r.c.Close()
				}
			}
		}()
	}
	launch()
	timer := time.NewTimer(raceDelay)
	defer timer.Stop()
	var firstErr error
	for {
		select {
		case r := <-ch:
			pending--
			if r.err == nil {
				discard(pending)
				return tune(r.c), nil
			}
			if firstErr == nil {
				firstErr = r.err
			}
			if launched < len(addrs) {
				launch()
				timer.Reset(raceDelay)
			} else if pending == 0 {
				return nil, firstErr
			}
		case <-timer.C:
			if launched < len(addrs) {
				launch()
				timer.Reset(raceDelay)
			}
		case <-ctx.Done():
			discard(pending)
			return nil, ctx.Err()
		}
	}
}
