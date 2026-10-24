package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/app/instancesettings"
	"github.com/bitan-del/dakiya-mail/internal/pkg/generation"
)

type fixedSettingsStore struct{ doc instancesettings.Document }

func (s *fixedSettingsStore) Get(context.Context) (instancesettings.Document, error) {
	return s.doc, nil
}
func (s *fixedSettingsStore) Put(context.Context, instancesettings.Document, *uuid.UUID) error {
	return nil
}

// fakeProvider answers like an OpenAI-compatible endpoint and records the bearer it was given.
func fakeProvider(t *testing.T, status int, body string) (url string, bearer *string) {
	t.Helper()
	got := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Get("Authorization")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

const okCompletion = `{"choices":[{"message":{"content":"ok"}}],"usage":{"total_tokens":3}}`

func TestTestAIProviderOK(t *testing.T) {
	url, bearer := fakeProvider(t, 200, okCompletion)
	res := testAIProvider(context.Background(), generation.ProviderSettings{Provider: "custom", APIKey: "k-secret", BaseURL: url, Model: "m1"})
	if res["ok"] != true || res["model"] != "m1" {
		t.Fatalf("got %v", res)
	}
	if *bearer != "Bearer k-secret" {
		t.Fatalf("bearer = %q", *bearer)
	}
}

func TestTestAIProviderNeverEchoesTheKey(t *testing.T) {
	url, _ := fakeProvider(t, 401, `{"error":{"type":"invalid_request_error","message":"Incorrect API key provided: k-secret"}}`)
	res := testAIProvider(context.Background(), generation.ProviderSettings{Provider: "custom", APIKey: "k-secret", BaseURL: url, Model: "m1"})
	msg, _ := res["error"].(string)
	if res["ok"] != false || msg == "" {
		t.Fatalf("got %v", res)
	}
	if strings.Contains(msg, "k-secret") || !strings.Contains(msg, "[redacted]") {
		t.Fatalf("the provider's message was not scrubbed: %q", msg)
	}
}

func TestTestAIProviderNeedsAKey(t *testing.T) {
	res := testAIProvider(context.Background(), generation.ProviderSettings{Provider: "openai"})
	if res["ok"] != false || res["error"] != "This provider needs an API key." {
		t.Fatalf("got %v", res)
	}
	res = testAIProvider(context.Background(), generation.ProviderSettings{})
	if res["ok"] != false || res["error"] != "No AI provider is configured on this instance." {
		t.Fatalf("got %v", res)
	}
}

func TestScrubSecretBoundsLength(t *testing.T) {
	if got := scrubSecret(strings.Repeat("x", 1000), ""); len([]rune(got)) != 403 {
		t.Fatalf("len = %d", len([]rune(got)))
	}
}

func newAITestRouter(saved instancesettings.AI, env generation.ProviderSettings) *gin.Engine {
	doc := instancesettings.Defaults()
	doc.AI = saved
	h := &Handler{AIEnv: env, InstanceSettings: instancesettings.NewService(&fixedSettingsStore{doc: doc})}
	r := gin.New()
	r.POST("/ai/test", h.AdminTestInstanceAI)
	return r
}

func postAITest(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/ai/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdminTestInstanceAIDraftReusesSavedKeyForTheSameProvider(t *testing.T) {
	url, bearer := fakeProvider(t, 200, okCompletion)
	r := newAITestRouter(instancesettings.AI{Provider: "custom", APIKey: "saved-key", BaseURL: url, Model: "m"}, generation.ProviderSettings{})

	w := postAITest(r, `{"provider":"custom","base_url":"`+url+`","model":"m2","api_key":""}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if *bearer != "Bearer saved-key" {
		t.Fatalf("bearer = %q", *bearer)
	}
	if strings.Contains(w.Body.String(), "saved-key") {
		t.Fatal("the response leaked the key")
	}
}

func TestAdminTestInstanceAIDraftNeverSendsASavedKeyToAnotherProvider(t *testing.T) {
	url, bearer := fakeProvider(t, 200, okCompletion)
	r := newAITestRouter(instancesettings.AI{Provider: "openai", APIKey: "saved-key"}, generation.ProviderSettings{})

	w := postAITest(r, `{"provider":"custom","base_url":"`+url+`","model":"m"}`)
	if !strings.Contains(w.Body.String(), `"ok":false`) {
		t.Fatalf("%s", w.Body)
	}
	if *bearer != "" {
		t.Fatalf("a request went out carrying %q", *bearer)
	}
}

func TestAdminTestInstanceAIEmptyBodyTestsSavedOverEnv(t *testing.T) {
	url, bearer := fakeProvider(t, 200, okCompletion)
	env := generation.ProviderSettings{Provider: "openai", APIKey: "env-key"}
	r := newAITestRouter(instancesettings.AI{Provider: "custom", APIKey: "saved-key", BaseURL: url, Model: "m"}, env)

	w := postAITest(r, ``)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if *bearer != "Bearer saved-key" {
		t.Fatalf("bearer = %q", *bearer)
	}
}
