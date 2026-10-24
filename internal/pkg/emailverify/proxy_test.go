package emailverify

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseProxyURL(t *testing.T) {
	cases := []struct {
		raw      string
		wantAddr string
		wantUser string
		wantPass string
		wantErr  bool
	}{
		{raw: "", wantAddr: ""}, // unset env var is not an error
		{raw: "socks5://proxy.example.com:1080", wantAddr: "proxy.example.com:1080"},
		{raw: "socks5h://proxy.example.com:1080", wantAddr: "proxy.example.com:1080"},
		{raw: "socks5://u:p@proxy.example.com:1080", wantAddr: "proxy.example.com:1080", wantUser: "u", wantPass: "p"},
		{raw: "  socks5://proxy.example.com:1080  ", wantAddr: "proxy.example.com:1080"},
		{raw: "http://proxy.example.com:8080", wantErr: true}, // wrong scheme
		{raw: "socks5://proxy.example.com", wantErr: true},    // no port
		{raw: "socks5://", wantErr: true},                     // no host
	}
	for _, c := range cases {
		got, err := ParseProxyURL(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseProxyURL(%q) = %+v, want error", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProxyURL(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if got.Address != c.wantAddr || got.Username != c.wantUser || got.Password != c.wantPass {
			t.Errorf("ParseProxyURL(%q) = %+v, want addr=%q user=%q pass=%q",
				c.raw, got, c.wantAddr, c.wantUser, c.wantPass)
		}
		if got.Enabled() != (c.wantAddr != "") {
			t.Errorf("ParseProxyURL(%q).Enabled() = %v", c.raw, got.Enabled())
		}
	}
}

// A configured-but-unreachable proxy must fail the probe, never quietly fall
// back to a direct dial. The whole point of the proxy is to keep RCPT traffic
// off the sending IPs, so a silent bypass would be worse than no probe at all.
//
// The MX here is real and would answer 250 on a direct dial, so a pass proves
// the probe did not go around the proxy.
func TestProbeDoesNotBypassAnUnreachableProxy(t *testing.T) {
	mx := newFakeMX(t, "250 2.1.5 Ok")

	// Bind then immediately close, so the port is almost certainly dead.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := ln.Addr().String()
	_ = ln.Close()

	v := testVerifier(t, Config{
		HeloHost: "verify.dakiyamail.com",
		Proxy:    ProxyConfig{Address: dead},
	}, mx.port())

	res := v.probe(context.Background(), "127.0.0.1", "ok", "example.com")
	if res.outcome == probeAccepted {
		t.Fatal("probe succeeded with a dead proxy — it bypassed the proxy and dialed direct")
	}
	if res.outcome != probeUnknown {
		t.Fatalf("outcome = %v, want probeUnknown for an unreachable proxy", res.outcome)
	}
	if !strings.Contains(res.reason, "proxy") {
		t.Errorf("reason = %q, want it to name the proxy so the cause is diagnosable", res.reason)
	}
}

func TestDialerForDirectWhenUnset(t *testing.T) {
	d, err := dialerFor(ProxyConfig{}, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := d.(*net.Dialer); !ok {
		t.Errorf("expected a direct *net.Dialer, got %T", d)
	}
}

// socks5Echo is a minimal SOCKS5 server: it completes the no-auth handshake,
// accepts a CONNECT, dials the requested target itself, and pipes bytes. Enough
// to prove the probe really egresses through the proxy rather than dialing out
// directly.
func socks5Echo(t *testing.T) (addr string, connects *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var count atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := make([]byte, 2)
				if _, err := io.ReadFull(c, br); err != nil {
					return
				}
				nMethods := int(br[1])
				if _, err := io.ReadFull(c, make([]byte, nMethods)); err != nil {
					return
				}
				// version 5, method 0 (no auth)
				if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
					return
				}
				hdr := make([]byte, 4)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				var host string
				switch hdr[3] {
				case 0x01: // IPv4
					b := make([]byte, 4)
					if _, err := io.ReadFull(c, b); err != nil {
						return
					}
					host = net.IP(b).String()
				case 0x03: // domain
					l := make([]byte, 1)
					if _, err := io.ReadFull(c, l); err != nil {
						return
					}
					b := make([]byte, int(l[0]))
					if _, err := io.ReadFull(c, b); err != nil {
						return
					}
					host = string(b)
				default:
					return
				}
				pb := make([]byte, 2)
				if _, err := io.ReadFull(c, pb); err != nil {
					return
				}
				target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
				up, err := net.DialTimeout("tcp", target, 3*time.Second)
				if err != nil {
					_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					return
				}
				defer func() { _ = up.Close() }()
				count.Add(1)
				// success reply
				if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().String(), &count
}

// The probe, pointed at a SOCKS5 proxy, must reach the MX through it.
func TestProbeGoesThroughSOCKS5(t *testing.T) {
	mx := newFakeMX(t, "250 2.1.5 Ok")
	proxyAddr, connects := socks5Echo(t)

	v := testVerifier(t, Config{
		HeloHost: "verify.dakiyamail.com",
		Proxy:    ProxyConfig{Address: proxyAddr},
	}, mx.port())

	res := v.probe(context.Background(), "127.0.0.1", "ok", "example.com")
	if res.outcome != probeAccepted {
		t.Fatalf("outcome = %v, reason = %q; want accepted via proxy", res.outcome, res.reason)
	}
	if connects.Load() == 0 {
		t.Error("the proxy saw no CONNECT — the probe dialed directly")
	}
	if strings.Contains(res.reason, "proxy") {
		t.Errorf("unexpected proxy error in reason: %q", res.reason)
	}
}
