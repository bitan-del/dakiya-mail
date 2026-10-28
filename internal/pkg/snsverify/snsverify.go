// Package snsverify checks that an HTTP(S) delivery really came from Amazon SNS:
// the signing certificate must be served by an sns.<region>.amazonaws.com host
// and the message signature must verify against it.
package snsverify

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SNS SignatureVersion 1 is defined over SHA-1.
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Message is the JSON envelope SNS posts for every message type.
type Message struct {
	Type             string `json:"Type"`
	MessageID        string `json:"MessageId"`
	Token            string `json:"Token,omitempty"`
	TopicArn         string `json:"TopicArn"`
	Subject          string `json:"Subject,omitempty"`
	Message          string `json:"Message"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
	Signature        string `json:"Signature"`
	SigningCertURL   string `json:"SigningCertURL"`
	SubscribeURL     string `json:"SubscribeURL,omitempty"`
}

const (
	TypeNotification             = "Notification"
	TypeSubscriptionConfirmation = "SubscriptionConfirmation"
	TypeUnsubscribeConfirmation  = "UnsubscribeConfirmation"
)

var snsHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

// ErrInvalid is returned for any message that does not verify.
var ErrInvalid = errors.New("sns message failed verification")

// IsSNSURL reports whether raw is an https URL on an SNS endpoint host.
func IsSNSURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return snsHost.MatchString(strings.ToLower(u.Hostname()))
}

// Verifier fetches and caches SNS signing certificates.
type Verifier struct {
	client *http.Client
	mu     sync.Mutex
	certs  map[string]*x509.Certificate
}

func New() *Verifier {
	return &Verifier{
		client: &http.Client{Timeout: 5 * time.Second},
		certs:  map[string]*x509.Certificate{},
	}
}

func (v *Verifier) Verify(ctx context.Context, m *Message) error {
	if m == nil || !IsSNSURL(m.SigningCertURL) || !strings.HasSuffix(m.SigningCertURL, ".pem") {
		return ErrInvalid
	}
	var h crypto.Hash
	switch m.SignatureVersion {
	case "1":
		h = crypto.SHA1
	case "2":
		h = crypto.SHA256
	default:
		return ErrInvalid
	}
	payload, err := stringToSign(m)
	if err != nil {
		return ErrInvalid
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return ErrInvalid
	}
	cert, err := v.cert(ctx, m.SigningCertURL)
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return ErrInvalid
	}
	var digest []byte
	if h == crypto.SHA1 {
		d := sha1.Sum([]byte(payload)) //nolint:gosec // required by SignatureVersion 1
		digest = d[:]
	} else {
		d := sha256.Sum256([]byte(payload))
		digest = d[:]
	}
	if err := rsa.VerifyPKCS1v15(pub, h, digest, sig); err != nil {
		return ErrInvalid
	}
	return nil
}

func stringToSign(m *Message) (string, error) {
	var b strings.Builder
	add := func(k, v string) {
		b.WriteString(k)
		b.WriteByte('\n')
		b.WriteString(v)
		b.WriteByte('\n')
	}
	switch m.Type {
	case TypeNotification:
		add("Message", m.Message)
		add("MessageId", m.MessageID)
		if m.Subject != "" {
			add("Subject", m.Subject)
		}
		add("Timestamp", m.Timestamp)
		add("TopicArn", m.TopicArn)
		add("Type", m.Type)
	case TypeSubscriptionConfirmation, TypeUnsubscribeConfirmation:
		add("Message", m.Message)
		add("MessageId", m.MessageID)
		add("SubscribeURL", m.SubscribeURL)
		add("Timestamp", m.Timestamp)
		add("Token", m.Token)
		add("TopicArn", m.TopicArn)
		add("Type", m.Type)
	default:
		return "", fmt.Errorf("unknown sns message type %q", m.Type)
	}
	return b.String(), nil
}

func (v *Verifier) cert(ctx context.Context, certURL string) (*x509.Certificate, error) {
	v.mu.Lock()
	if c, ok := v.certs[certURL]; ok {
		v.mu.Unlock()
		return c, nil
	}
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, ErrInvalid
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch sns signing cert: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch sns signing cert: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("read sns signing cert: %w", err)
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, ErrInvalid
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, ErrInvalid
	}
	if time.Now().After(cert.NotAfter) {
		return nil, ErrInvalid
	}

	v.mu.Lock()
	if len(v.certs) > 64 {
		v.certs = map[string]*x509.Certificate{}
	}
	v.certs[certURL] = cert
	v.mu.Unlock()
	return cert, nil
}

// Confirm visits SubscribeURL to accept a subscription. Only SNS hosts are fetched.
func (v *Verifier) Confirm(ctx context.Context, m *Message) error {
	if !IsSNSURL(m.SubscribeURL) {
		return ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.SubscribeURL, nil)
	if err != nil {
		return ErrInvalid
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("confirm sns subscription: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("confirm sns subscription: status %d", resp.StatusCode)
	}
	return nil
}
