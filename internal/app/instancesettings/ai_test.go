package instancesettings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/pkg/encrypt"
)

const testKey = "sk-live-0123456789abcdefWXYZ"

func patchOf(t *testing.T, body string) Patch {
	t.Helper()
	var p Patch
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return p
}

func testEncrypter(t *testing.T) *encrypt.Encrypter {
	t.Helper()
	enc, err := encrypt.NewEncrypter(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestAIKeyIsNeverSerializedToClients(t *testing.T) {
	doc := Defaults()
	doc.AI = AI{Provider: "openai", APIKey: testKey}
	doc.Normalize()

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testKey) || strings.Contains(string(raw), "sealed") {
		t.Fatalf("the document leaked the key: %s", raw)
	}
	var out struct {
		AI struct {
			Set  bool   `json:"api_key_set"`
			Hint string `json:"api_key_hint"`
		} `json:"ai"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.AI.Set || out.AI.Hint != "WXYZ" {
		t.Fatalf("want set with the last four characters, got %+v", out.AI)
	}
}

func TestAIKeyHintMasking(t *testing.T) {
	tests := []struct {
		key  string
		set  bool
		hint string
	}{
		{"", false, ""},
		{"short", true, ""},
		{"elevenchars", true, ""},
		{"twelve-chars", true, "hars"},
		{testKey, true, "WXYZ"},
	}
	for _, tt := range tests {
		a := AI{APIKey: tt.key}
		a.Normalize()
		if a.APIKeySet != tt.set || a.APIKeyHint != tt.hint {
			t.Errorf("key %q: set=%v hint=%q, want set=%v hint=%q", tt.key, a.APIKeySet, a.APIKeyHint, tt.set, tt.hint)
		}
	}
}

func TestPatchAIKeepClearReplace(t *testing.T) {
	stored := Defaults()
	stored.AI = AI{Provider: "openai", Model: "gpt-4o", APIKey: testKey}

	t.Run("absent key keeps it", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"provider":"openai","model":"gpt-4o-mini"}}`).Apply(stored)
		if got.AI.APIKey != testKey || got.AI.Model != "gpt-4o-mini" {
			t.Fatalf("got %+v", got.AI)
		}
	})
	t.Run("empty key keeps it", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"api_key":"   "}}`).Apply(stored)
		if got.AI.APIKey != testKey {
			t.Fatal("an empty key must keep the stored one")
		}
	})
	t.Run("an unrelated section keeps it", func(t *testing.T) {
		got := patchOf(t, `{"access":{"allow_invited_signup":false}}`).Apply(stored)
		if got.AI.APIKey != testKey || got.AI.Provider != "openai" {
			t.Fatalf("got %+v", got.AI)
		}
	})
	t.Run("clear flag removes it", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"clear_api_key":true}}`).Apply(stored)
		got.Normalize()
		if got.AI.APIKey != "" || got.AI.APIKeySet {
			t.Fatalf("key survived a clear: %+v", got.AI)
		}
		if got.AI.Provider != "openai" || got.AI.Model != "gpt-4o" {
			t.Fatalf("clearing the key touched other fields: %+v", got.AI)
		}
	})
	t.Run("a new key replaces it", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"api_key":" sk-new "}}`).Apply(stored)
		if got.AI.APIKey != "sk-new" {
			t.Fatalf("got %q", got.AI.APIKey)
		}
	})
	t.Run("changing provider without a key drops the old key", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"provider":"GROQ"}}`).Apply(stored)
		if got.AI.Provider != "groq" || got.AI.APIKey != "" {
			t.Fatalf("an openai key must not follow the operator to groq: %+v", got.AI)
		}
	})
	t.Run("changing provider with a key stores the new key", func(t *testing.T) {
		got := patchOf(t, `{"ai":{"provider":"groq","api_key":"gsk-new"}}`).Apply(stored)
		if got.AI.Provider != "groq" || got.AI.APIKey != "gsk-new" {
			t.Fatalf("got %+v", got.AI)
		}
	})
}

func TestAIValidate(t *testing.T) {
	tests := []struct {
		name string
		ai   AI
		ok   bool
	}{
		{"empty is the environment", AI{}, true},
		{"preset", AI{Provider: "groq", APIKey: "gsk-1"}, true},
		{"unknown provider", AI{Provider: "skynet"}, false},
		{"custom needs a base url", AI{Provider: "custom", Model: "m"}, false},
		{"custom needs a model", AI{Provider: "custom", BaseURL: "http://vllm:8000/v1"}, false},
		{"custom complete", AI{Provider: "custom", BaseURL: "http://vllm:8000/v1", Model: "m"}, true},
		{"base url must be http(s)", AI{Provider: "openai", BaseURL: "ftp://x"}, false},
		{"base url must not carry credentials", AI{Provider: "openai", BaseURL: "https://u:p@x"}, false},
		{"key must be one token", AI{Provider: "openai", APIKey: "sk a"}, false},
		{"key must not hold a line break", AI{Provider: "openai", APIKey: "sk-a\nb"}, false},
	}
	for _, tt := range tests {
		err := tt.ai.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("%s: err = %v, want ok=%v", tt.name, err, tt.ok)
		}
		var invalid *InvalidError
		if err != nil && !errors.As(err, &invalid) {
			t.Errorf("%s: %v is not an InvalidError", tt.name, err)
		}
	}
}

func TestSealedKeyRoundTrip(t *testing.T) {
	enc := testEncrypter(t)
	doc := Defaults()
	doc.AI = AI{Provider: "openai", Model: "gpt-4o", APIKey: testKey}

	raw, err := encodeDocument(doc, enc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testKey) {
		t.Fatalf("the stored document holds the key in plaintext: %s", raw)
	}
	if !strings.Contains(string(raw), "api_key_sealed") {
		t.Fatalf("the stored document has no sealed key: %s", raw)
	}

	back, err := decodeDocument(raw, enc)
	if err != nil {
		t.Fatal(err)
	}
	if back.AI.APIKey != testKey || back.AI.Provider != "openai" || !back.AI.APIKeySet {
		t.Fatalf("round trip lost data: %+v", back.AI)
	}
}

func TestSavingWithoutAnEncrypterRefusesAKey(t *testing.T) {
	doc := Defaults()
	doc.AI = AI{Provider: "openai", APIKey: testKey}
	_, err := encodeDocument(doc, nil)
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("want an InvalidError naming CREDENTIALS_ENCRYPTION_KEY, got %v", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatal("the error leaked the key")
	}
}

// A key this process cannot read must survive an unrelated save, not be wiped by it.
func TestUnreadableKeyIsPreservedAcrossAnUnrelatedSave(t *testing.T) {
	enc := testEncrypter(t)
	doc := Defaults()
	doc.AI = AI{Provider: "openai", APIKey: testKey}
	raw, err := encodeDocument(doc, enc)
	if err != nil {
		t.Fatal(err)
	}

	blind, err := decodeDocument(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if blind.AI.APIKey != "" || blind.AI.APIKeySet {
		t.Fatalf("a key sealed with a missing cipher must read as unset: %+v", blind.AI)
	}

	edited := patchOf(t, `{"access":{"allow_invited_signup":false}}`).Apply(blind)
	again, err := encodeDocument(edited, nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeDocument(again, enc)
	if err != nil {
		t.Fatal(err)
	}
	if back.AI.APIKey != testKey {
		t.Fatal("an unrelated save dropped the sealed key")
	}

	// An explicit clear still wins over preservation.
	cleared := patchOf(t, `{"ai":{"clear_api_key":true}}`).Apply(blind)
	raw, err = encodeDocument(cleared, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "api_key_sealed") {
		t.Fatalf("a cleared key left ciphertext behind: %s", raw)
	}
}

// A document stored before the AI section existed must read as "use the environment".
func TestOlderDocumentHasNoSavedAI(t *testing.T) {
	doc, err := decodeDocument([]byte(`{"invitations":{"links_enabled":false,"ttl_hours":24}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.AI.Provider != "" || doc.AI.APIKeySet {
		t.Fatalf("unexpected saved AI: %+v", doc.AI)
	}
	if doc.Invitations.TTLHours != 24 {
		t.Fatal("the rest of the document must still decode")
	}
}

type memStore struct{ doc Document }

func (m *memStore) Get(context.Context) (Document, error) { return m.doc, nil }
func (m *memStore) Put(_ context.Context, doc Document, _ *uuid.UUID) error {
	m.doc = doc
	return nil
}

func TestServicePutAI(t *testing.T) {
	ctx := context.Background()
	svc := NewService(&memStore{doc: Defaults()})

	if _, err := svc.Put(ctx, patchOf(t, `{"ai":{"provider":"groq","model":"m","api_key":"gsk-0123456789abcdef"}}`), nil); err != nil {
		t.Fatal(err)
	}
	if got := svc.AI(ctx); got.Provider != "groq" || got.APIKey != "gsk-0123456789abcdef" || got.APIKeyHint != "cdef" {
		t.Fatalf("got %+v", got)
	}

	// A save of another section keeps the key; a rejected value changes nothing.
	if _, err := svc.Put(ctx, patchOf(t, `{"ai":{"model":"m2"}}`), nil); err != nil {
		t.Fatal(err)
	}
	if got := svc.AI(ctx); got.APIKey == "" || got.Model != "m2" {
		t.Fatalf("got %+v", got)
	}
	if _, err := svc.Put(ctx, patchOf(t, `{"ai":{"provider":"nope"}}`), nil); err == nil {
		t.Fatal("an unknown provider must be rejected")
	}
	if got := svc.AI(ctx); got.Provider != "groq" {
		t.Fatalf("a rejected save changed the settings: %+v", got)
	}

	if _, err := svc.Put(ctx, patchOf(t, `{"ai":{"clear_api_key":true}}`), nil); err != nil {
		t.Fatal(err)
	}
	if got := svc.AI(ctx); got.APIKeySet || got.Provider != "groq" {
		t.Fatalf("got %+v", got)
	}
}
