package generation

import (
	"context"
	"testing"
)

type fakeEnv map[string]string

func (f fakeEnv) GetStringOptional(_ context.Context, envKey, _, def string) string {
	if v := f[envKey]; v != "" {
		return v
	}
	return def
}

func (f fakeEnv) GetSecretOptional(ctx context.Context, envKey, awsKey, def string) string {
	return f.GetStringOptional(ctx, envKey, awsKey, def)
}

func (f fakeEnv) GetBoolPtr(_ context.Context, envKey, _ string) *bool {
	switch f[envKey] {
	case "true":
		b := true
		return &b
	case "false":
		b := false
		return &b
	}
	return nil
}

func TestEnvSettingsNormalizesProvider(t *testing.T) {
	got := EnvSettings(context.Background(), fakeEnv{"AI_PROVIDER": "  OpenRouter ", "AI_API_KEY": "sk-env", "AI_FREE": "true"})
	if got.Provider != "openrouter" || got.APIKey != "sk-env" {
		t.Fatalf("got provider %q key %q", got.Provider, got.APIKey)
	}
	if got.Free == nil || !*got.Free {
		t.Fatal("AI_FREE=true should be read")
	}
}

func TestWithSavedPrefersSavedOverEnv(t *testing.T) {
	env := ProviderSettings{
		Provider: "openai", APIKey: "sk-env", BaseURL: "https://env.example/v1",
		Model: "env-model", ModelTrial: "env-trial", ModelPaid: "env-paid",
	}
	got := env.WithSaved(" Groq ", "gsk-saved", "", "saved-model")

	if got.Provider != "groq" || got.APIKey != "gsk-saved" || got.Model != "saved-model" {
		t.Fatalf("saved values did not win: %+v", got)
	}
	// The env endpoint and tier overrides must not leak into the saved provider.
	if got.BaseURL != "" || got.ModelTrial != "" || got.ModelPaid != "" {
		t.Fatalf("env endpoint or tier models leaked through: %+v", got)
	}

	cfg, err := Resolve(got)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.OpenAIAPIKey != "gsk-saved" || cfg.OpenAIBaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("resolved against the wrong host or key: %+v", cfg)
	}
	if cfg.OpenAIModelTrial != "saved-model" || cfg.OpenAIModelPaid != "saved-model" {
		t.Fatalf("saved model should set both tiers: %+v", cfg)
	}
}

func TestWithSavedWithoutKeyNeverInheritsEnvKey(t *testing.T) {
	env := ProviderSettings{Provider: "openai", APIKey: "sk-env"}
	got := env.WithSaved("ollama", "", "", "")
	if got.APIKey != "" {
		t.Fatalf("env key leaked into a saved provider: %q", got.APIKey)
	}
}

func TestWithSavedEmptyProviderKeepsEnv(t *testing.T) {
	env := ProviderSettings{Provider: "openai", APIKey: "sk-env", Model: "env-model"}
	got := env.WithSaved("", "ignored", "https://ignored", "ignored")
	if got != env {
		t.Fatalf("empty saved provider must leave the environment alone: %+v", got)
	}
}
