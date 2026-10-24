package instancesettings

import (
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// AIProviders are the provider presets the AI section accepts, mirroring generation.presetFor.
var AIProviders = []string{"openai", "openrouter", "groq", "anthropic", "ollama", "custom"}

const (
	aiFieldMax   = 300
	aiKeyMax     = 512
	aiHintMinKey = 12
)

// InvalidError is a rejected settings value; its text is safe to show the operator.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// AI is the saved LLM provider. Unlike every other key here an environment
// variable (AI_PROVIDER and friends) also owns it: a saved provider wins and an
// empty one defers to the environment.
type AI struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	// APIKey is the plaintext key, in memory only: the store seals it at rest and no client ever receives it.
	APIKey string `json:"-"`
	// sealed is the stored ciphertext, kept so a save never drops a key this process could not read.
	sealed string
	// APIKeySet and APIKeyHint are derived in Normalize and are all a client ever learns of the key.
	APIKeySet  bool   `json:"api_key_set"`
	APIKeyHint string `json:"api_key_hint"`
}

// Normalize derives the client-visible view of the key: whether one is set and its last four characters.
func (a *AI) Normalize() {
	a.APIKeySet = a.APIKey != ""
	a.APIKeyHint = ""
	if r := []rune(a.APIKey); len(r) >= aiHintMinKey {
		a.APIKeyHint = string(r[len(r)-4:])
	}
}

// Validate rejects an unknown provider, a malformed endpoint or a custom provider missing what it needs.
func (a AI) Validate() error {
	if a.Provider != "" && !slices.Contains(AIProviders, a.Provider) {
		return &InvalidError{"Unknown AI provider."}
	}
	if len(a.Model) > aiFieldMax || len(a.BaseURL) > aiFieldMax {
		return &InvalidError{"The model and base URL must be under 300 characters."}
	}
	if len(a.APIKey) > aiKeyMax || strings.ContainsFunc(a.APIKey, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return &InvalidError{"The API key must be a single token with no spaces or line breaks."}
	}
	if a.BaseURL != "" {
		u, err := url.Parse(a.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return &InvalidError{"The base URL must be an http(s) URL without credentials."}
		}
	}
	if a.Provider == "custom" && (a.BaseURL == "" || a.Model == "") {
		return &InvalidError{"A custom provider needs a base URL and a model."}
	}
	return nil
}

// AIPatch is the AI part of a partial update. Absent fields keep their stored value.
type AIPatch struct {
	Provider *string `json:"provider"`
	Model    *string `json:"model"`
	BaseURL  *string `json:"base_url"`
	// APIKey replaces the stored key when non-empty; empty or absent keeps it.
	APIKey *string `json:"api_key"`
	// ClearAPIKey removes the stored key.
	ClearAPIKey *bool `json:"clear_api_key"`
}

// ChangesKey reports whether the patch sets or removes the stored key.
func (p *AIPatch) ChangesKey() bool {
	if p == nil {
		return false
	}
	return (p.APIKey != nil && strings.TrimSpace(*p.APIKey) != "") || (p.ClearAPIKey != nil && *p.ClearAPIKey)
}

func (p *AIPatch) apply(a AI) AI {
	if p == nil {
		return a
	}
	if p.Provider != nil {
		next := strings.ToLower(strings.TrimSpace(*p.Provider))
		// A key belongs to one provider: switching without a new key drops it rather than sending it to another host.
		if next != a.Provider {
			a.APIKey, a.sealed = "", ""
		}
		a.Provider = next
	}
	if p.Model != nil {
		a.Model = strings.TrimSpace(*p.Model)
	}
	if p.BaseURL != nil {
		a.BaseURL = strings.TrimSpace(*p.BaseURL)
	}
	if p.ClearAPIKey != nil && *p.ClearAPIKey {
		a.APIKey, a.sealed = "", ""
	}
	if p.APIKey != nil {
		if key := strings.TrimSpace(*p.APIKey); key != "" {
			a.APIKey = key
		}
	}
	return a
}
