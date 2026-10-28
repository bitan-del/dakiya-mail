package snsverify

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // exercising SignatureVersion 1
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"testing"
	"time"
)

const certURL = "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem"

func signed(t *testing.T, key *rsa.PrivateKey, m *Message) *Message {
	t.Helper()
	payload, err := stringToSign(m)
	if err != nil {
		t.Fatal(err)
	}
	var sig []byte
	if m.SignatureVersion == "1" {
		d := sha1.Sum([]byte(payload)) //nolint:gosec
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, d[:])
	} else {
		d := sha256.Sum256([]byte(payload))
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	}
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	return m
}

func testVerifier(t *testing.T) (*Verifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	v := New()
	v.certs[certURL] = cert
	return v, key
}

func notification(version string) *Message {
	return &Message{
		Type:             TypeNotification,
		MessageID:        "1b2c",
		TopicArn:         "arn:aws:sns:us-east-1:123456789012:ses-events",
		Message:          `{"notificationType":"Delivery"}`,
		Timestamp:        "2026-10-28T10:00:00.000Z",
		SignatureVersion: version,
		SigningCertURL:   certURL,
	}
}

func TestVerify(t *testing.T) {
	v, key := testVerifier(t)
	ctx := context.Background()

	for _, ver := range []string{"1", "2"} {
		if err := v.Verify(ctx, signed(t, key, notification(ver))); err != nil {
			t.Fatalf("v%s: valid message rejected: %v", ver, err)
		}
	}

	tampered := signed(t, key, notification("2"))
	tampered.Message = `{"notificationType":"Bounce"}`
	if err := v.Verify(ctx, tampered); err == nil {
		t.Fatal("tampered message accepted")
	}

	sub := signed(t, key, &Message{
		Type: TypeSubscriptionConfirmation, MessageID: "m", Token: "tok", TopicArn: "arn:t",
		Message: "confirm", Timestamp: "2026-10-28T10:00:00.000Z", SignatureVersion: "2",
		SigningCertURL: certURL, SubscribeURL: "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription",
	})
	if err := v.Verify(ctx, sub); err != nil {
		t.Fatalf("subscription confirmation rejected: %v", err)
	}
}

func TestIsSNSURL(t *testing.T) {
	good := []string{
		"https://sns.us-east-1.amazonaws.com/cert.pem",
		"https://sns.cn-north-1.amazonaws.com.cn/cert.pem",
	}
	bad := []string{
		"http://sns.us-east-1.amazonaws.com/cert.pem",
		"https://sns.us-east-1.amazonaws.com.evil.com/cert.pem",
		"https://evil.com/sns.us-east-1.amazonaws.com/cert.pem",
		"https://sns.us-east-1.amazonaws.com:8443/cert.pem",
		"https://user@sns.us-east-1.amazonaws.com/cert.pem",
	}
	for _, u := range good {
		if !IsSNSURL(u) {
			t.Errorf("IsSNSURL(%q) = false", u)
		}
	}
	for _, u := range bad {
		if IsSNSURL(u) {
			t.Errorf("IsSNSURL(%q) = true", u)
		}
	}
}
