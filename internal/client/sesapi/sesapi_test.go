package sesapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegion(t *testing.T) {
	if r, ok := Region("email.eu-west-1.amazonaws.com"); !ok || r != "eu-west-1" {
		t.Errorf("Region(api host) = %q, %v", r, ok)
	}
	for _, h := range []string{"email-smtp.us-east-1.amazonaws.com", "smtp.gmail.com", "email.us-east-1.amazonaws.com.evil.com"} {
		if _, ok := Region(h); ok {
			t.Errorf("Region(%q) matched", h)
		}
	}
}

func TestSendAgainstFakeSES(t *testing.T) {
	var got struct {
		FromEmailAddress string
		Destination      struct{ ToAddresses []string }
		Content          struct{ Raw struct{ Data []byte } }
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIATEST/") {
			t.Errorf("request not signed with the mailbox's key: %q", r.Header.Get("Authorization"))
		}
		_ = json.Unmarshal(body, &got)
		if strings.Contains(got.FromEmailAddress, "unverified") {
			w.Header().Set("X-Amzn-ErrorType", "MessageRejected")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Email address is not verified."}`))
			return
		}
		_, _ = w.Write([]byte(`{"MessageId":"0100018c-test-000000"}`))
	}))
	defer srv.Close()
	endpointOverride = srv.URL
	defer func() { endpointOverride = "" }()

	raw := []byte("Subject: hi\r\n\r\nbody\r\n")
	id, err := Send(context.Background(), "us-east-1", "AKIATEST", "secret", nil, "orders@example.com", []string{"a@example.com", "b@example.com"}, raw)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "0100018c-test-000000" {
		t.Errorf("message id = %q", id)
	}
	if got.FromEmailAddress != "orders@example.com" || len(got.Destination.ToAddresses) != 2 || string(got.Content.Raw.Data) != string(raw) {
		t.Errorf("request = %+v", got)
	}

	_, err = Send(context.Background(), "us-east-1", "AKIATEST", "secret", nil, "unverified@example.com", []string{"a@example.com"}, raw)
	var se *SendError
	if !errors.As(err, &se) || se.Kind != FailureSender {
		t.Fatalf("unverified sender should be a sender failure, got %v", err)
	}
}
