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
}

// DefaultConfig returns sensible defaults (similar to IDM: 8 connections).
func DefaultConfig() Config {
	return Config{
		MaxConns:   8,
		MinSplit:   1 << 20, // 1 MiB
		BufSize:    256 << 10,
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
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: c.Timeout,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true, // byte ranges must address the raw entity
		ForceAttemptHTTP2:     c.AllowHTTP2,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: c.InsecureTLS}, //nolint:gosec // user option
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
