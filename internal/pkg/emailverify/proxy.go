package emailverify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// SOCKS5 egress for the RCPT probe.
//
// Verification needs an outbound connection to the recipient's MX on port 25,
// and most cloud providers (AWS, GCP, Azure, DigitalOcean, Hetzner) block that
// by default and will not lift it for a new account. Without a route out, every
// address a probe touches comes back "unknown" — the check is inert.
//
// Routing the probe through a SOCKS5 proxy that does have :25 open is the usual
// fix. It also keeps probe traffic off the IPs the campaigns send from, which
// matters independently: a burst of RCPT probes from a sending IP looks like
// directory harvesting and is a good way to lose the reputation the whole
// warmup system exists to build.

// ProxyConfig points the probe at a SOCKS5 proxy. Zero value means direct.
type ProxyConfig struct {
	// Address is host:port of the SOCKS5 proxy.
	Address string
	// Username / Password are optional; leave empty for an unauthenticated proxy.
	Username string
	Password string
}

// Enabled reports whether a proxy is configured.
func (p ProxyConfig) Enabled() bool { return strings.TrimSpace(p.Address) != "" }

// ParseProxyURL builds a ProxyConfig from a socks5:// URL, the shape operators
// usually already have from other tooling:
//
//	socks5://user:pass@proxy.example.com:1080
//	socks5://proxy.example.com:1080
//
// An empty string yields a disabled config and no error, so callers can pass an
// unset env var straight through.
func ParseProxyURL(raw string) (ProxyConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ProxyConfig{}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("parse proxy url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
		// socks5h means "resolve the hostname at the proxy". x/net/proxy always
		// hands the hostname to the proxy, so both spellings behave the same
		// here; accept both rather than rejecting a URL that works elsewhere.
	default:
		return ProxyConfig{}, fmt.Errorf("proxy scheme %q is not supported; use socks5://", u.Scheme)
	}
	if u.Host == "" {
		return ProxyConfig{}, errors.New("proxy url has no host")
	}
	if u.Port() == "" {
		return ProxyConfig{}, errors.New("proxy url must include a port, e.g. socks5://host:1080")
	}
	cfg := ProxyConfig{Address: u.Host}
	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	return cfg, nil
}

// contextDialer is the subset of net.Dialer the probe needs, so a proxy can be
// swapped in without the probe caring which it got.
type contextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// dialerFor returns the dialer the probe should use: the SOCKS5 proxy when one
// is configured, a direct dialer otherwise.
func dialerFor(p ProxyConfig, timeout time.Duration) (contextDialer, error) {
	direct := &net.Dialer{Timeout: timeout}
	if !p.Enabled() {
		return direct, nil
	}
	var auth *proxy.Auth
	if p.Username != "" || p.Password != "" {
		auth = &proxy.Auth{User: p.Username, Password: p.Password}
	}
	// The timeout on `direct` bounds the hop to the proxy itself; the proxy's
	// own connect to the MX is bounded by the session deadline the caller sets.
	d, err := proxy.SOCKS5("tcp", p.Address, auth, direct)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer: %w", err)
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		// Every current x/net/proxy SOCKS5 dialer implements ContextDialer;
		// fail loudly rather than silently dialing direct and leaking probe
		// traffic out of the sending IP.
		return nil, errors.New("socks5 dialer does not support contexts")
	}
	return cd, nil
}
