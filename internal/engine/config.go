// Package engine implements FluxDM's multi-connection HTTP download engine with
// dynamic segmentation: whenever a connection becomes free, it takes over half of
// the largest remaining segment, so all connections stay busy until the end.
package engine

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	ntlmssp "github.com/Azure/go-ntlmssp"
	"golang.org/x/net/proxy"
)

// Config holds engine-wide settings.
type Config struct {
	MaxConns     int           // default connections per download (1..32)
	MinSplit     int64         // a segment is split only if each half is >= MinSplit
	BufSize      int           // read buffer per connection
	Retries      int           // retries per connection before giving up
	RetryDelay   time.Duration // base delay, doubled on each retry
	Timeout      time.Duration // response header / idle read timeout
	UserAgent    string        // default User-Agent when the request has none
	Proxy        string        // "", "http://host:port", "socks5://user:pass@host:port"
	AllowHTTP2   bool          // HTTP/2 multiplexes all ranges on ONE TCP connection (slower acceleration); off by default
	InsecureTLS  bool          // accept invalid certificates
	SpeedLimiter *Limiter      // global limiter shared by all downloads (may be nil)
	AutoConns    bool          // add connections while throughput keeps rising (up to 32)
	BlockSize    int64         // max bytes per range request (keep-alive reuse); 0 = whole segment
	SpreadIPs    bool          // spread connections over all IPs of the host (CDNs)

	// HostGate limits connections per host across all downloads (may be nil).
	HostGate *HostGate
	// MaxConnsPerHost caps the transport connections per host (0 = unlimited).
	MaxConnsPerHost int
	// FairWeight > 0 gives the download its own fair share of SpeedLimiter with
	// this weight (1 normal, 2 high priority, 0.5 low); 0 = common bucket.
	FairWeight float64
}

// maxBlockSize caps the adaptive block size (speed x 2 s).
const maxBlockSize = 64 << 20

// endgameSplit is the smallest segment half stolen near the end of a download.
const endgameSplit = 256 << 10

// sessionCache lets every transport resume TLS sessions (saves one round trip
// per new connection to the same server).
var sessionCache = tls.NewLRUClientSessionCache(256)

// DefaultConfig returns sensible defaults (similar to IDM: 8 connections).
func DefaultConfig() Config {
	return Config{
		MaxConns:   8,
		MinSplit:   1 << 20, // 1 MiB
		BufSize:    1 << 20,
		BlockSize:  8 << 20,
		AutoConns:  true,
		SpreadIPs:  true,
		Retries:    10,
		RetryDelay: 500 * time.Millisecond,
		Timeout:    20 * time.Second,
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	}
}

func (c *Config) normalize() {
	d := DefaultConfig()
	if c.MaxConns <= 0 {
		c.MaxConns = d.MaxConns
	}
	if c.MaxConns > 32 {
		c.MaxConns = 32
	}
	if c.MinSplit <= 0 {
		c.MinSplit = d.MinSplit
	}
	if c.BufSize <= 0 {
		c.BufSize = d.BufSize
	}
	if int64(c.BufSize) > c.MinSplit {
		c.BufSize = int(c.MinSplit)
	}
	if c.Retries < 0 {
		c.Retries = 0
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = d.RetryDelay
	}
	if c.Timeout <= 0 {
		c.Timeout = d.Timeout
	}
	if c.UserAgent == "" {
		c.UserAgent = d.UserAgent
	}
}

// NewHTTPClient builds the HTTP client used for downloads. With AllowHTTP2=false
// every concurrent range request gets its own TCP connection, which is what makes
// multi-connection acceleration effective on per-connection throttled servers.
func NewHTTPClient(c Config) (*http.Client, error) {
	c.normalize()
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	dial := (&fastDialer{d: dialer, spread: c.SpreadIPs}).DialContext
	maxPerHost := 0
	if c.MaxConnsPerHost > 0 {
		// headroom for probes and small requests made outside the host gate
		maxPerHost = c.MaxConnsPerHost + 4
	}
	tr := &http.Transport{
		DialContext:           dial,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: c.Timeout,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true, // byte ranges must address the raw entity
		ForceAttemptHTTP2:     c.AllowHTTP2,
		ReadBufferSize:        64 << 10,
		WriteBufferSize:       64 << 10,
		MaxConnsPerHost:       maxPerHost,
		TLSClientConfig: &tls.Config{ //nolint:gosec // InsecureSkipVerify is a user option
			InsecureSkipVerify: c.InsecureTLS,
			ClientSessionCache: sessionCache,
		},
	}
	if !c.AllowHTTP2 {
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	if c.Proxy != "" {
		u, err := url.Parse(c.Proxy)
		if err != nil {
			return nil, err
		}
		switch u.Scheme {
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if u.User != nil {
				pw, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: pw}
			}
			sd, err := proxy.SOCKS5("tcp", u.Host, auth, dialer)
			if err != nil {
				return nil, err
			}
			if cd, ok := sd.(proxy.ContextDialer); ok {
				tr.DialContext = cd.DialContext
			}
		default:
			tr.Proxy = http.ProxyURL(u)
		}
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}
	// Negotiator upgrades "Authorization: Basic" to NTLM/Negotiate when the
	// server asks for it (corporate proxies / intranet servers).
	return &http.Client{Transport: ntlmssp.Negotiator{RoundTripper: tr}}, nil
}
