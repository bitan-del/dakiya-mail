package smtp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/textproto"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

// The fake server below uses a self-signed cert; netbind reads this once.
func TestMain(m *testing.M) {
	os.Setenv("MAIL_TLS_INSECURE", "true")
	os.Exit(m.Run())
}

func TestEhloName(t *testing.T) {
	host := func(h string, err error) func() (string, error) {
		return func() (string, error) { return h, err }
	}
	tests := []struct {
		name     string
		override string
		hostname func() (string, error)
		want     string
	}{
		{"override wins over a dotted hostname", "mail.acme.com", host("box.internal.net", nil), "mail.acme.com"},
		{"override is trimmed", "  mail.acme.com \n", host("", nil), "mail.acme.com"},
		{"override with whitespace inside is ignored", "mail acme.com", host("box.internal.net", nil), "box.internal.net"},
		{"override with CRLF injection is ignored", "a.com\r\nMAIL FROM:<x>", host("", nil), ""},
		{"dotted hostname used", "", host("worker-1.acme.com", nil), "worker-1.acme.com"},
		{"bare hostname keeps Go's default", "", host("worker-1", nil), ""},
		{"hostname error keeps Go's default", "", host("", errors.New("boom")), ""},
		{"blank override falls through", "   ", host("w.acme.com", nil), "w.acme.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ehloName(tt.override, tt.hostname); got != tt.want {
				t.Fatalf("ehloName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSendRawEhloBeforeAndAfterStartTLS pins the wire behavior: the configured
// name is used for the first EHLO and for the one net/smtp re-sends after
// STARTTLS, with no second Hello call (which net/smtp rejects).
func TestSendRawEhloBeforeAndAfterStartTLS(t *testing.T) {
	t.Setenv(envEHLOHostname, "mail.example.com")
	port, ehlos := fakeSMTPServer(t)

	c := &Client{
		Email:       "a@example.com",
		AuthType:    models.AuthPlain,
		Credentials: &models.Service{Username: "u", Password: "p", Host: "127.0.0.1", Port: port},
	}
	reply, err := c.sendRaw(context.Background(), "a@example.com", []string{"b@example.com"}, []byte("Subject: x\r\n\r\nhi\r\n"))
	if err != nil {
		t.Fatalf("sendRaw: %v", err)
	}
	if reply != "Ok 0100018c-test-000000" {
		t.Fatalf("final DATA reply = %q", reply)
	}

	want := []string{"mail.example.com", "mail.example.com"}
	if got := ehlos(); !reflect.DeepEqual(got, want) {
		t.Fatalf("EHLO names = %v, want %v", got, want)
	}
}

// fakeSMTPServer accepts one connection, speaks just enough ESMTP (STARTTLS,
// AUTH, MAIL, RCPT, DATA, QUIT) to complete a send, and records every EHLO name.
func fakeSMTPServer(t *testing.T) (port int, ehlos func() []string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var seen []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		tp := textproto.NewConn(conn)
		tp.PrintfLine("220 fake ESMTP")
		secure := false
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			cmd := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(cmd, "EHLO "):
				mu.Lock()
				seen = append(seen, strings.TrimSpace(line[5:]))
				mu.Unlock()
				tp.PrintfLine("250-fake")
				if !secure {
					tp.PrintfLine("250-STARTTLS")
				}
				tp.PrintfLine("250 AUTH PLAIN")
			case cmd == "STARTTLS":
				tp.PrintfLine("220 go ahead")
				tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
				if tc.Handshake() != nil {
					return
				}
				tp = textproto.NewConn(tc)
				secure = true
			case strings.HasPrefix(cmd, "AUTH "):
				tp.PrintfLine("235 ok")
			case strings.HasPrefix(cmd, "MAIL "), strings.HasPrefix(cmd, "RCPT "):
				tp.PrintfLine("250 ok")
			case cmd == "DATA":
				tp.PrintfLine("354 go")
				if _, err := tp.ReadDotLines(); err != nil {
					return
				}
				tp.PrintfLine("250 Ok 0100018c-test-000000")
			case cmd == "QUIT":
				tp.PrintfLine("221 bye")
				return
			default:
				tp.PrintfLine("502 unsupported")
			}
		}
	}()
	t.Cleanup(func() { ln.Close(); <-done })

	return ln.Addr().(*net.TCPAddr).Port, func() []string {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("fake SMTP server did not finish")
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}
