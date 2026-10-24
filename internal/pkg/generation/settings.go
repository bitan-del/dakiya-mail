package generation

import (
	"context"
	"strings"
)

// EnvReader is the slice of config.Config that provider selection reads.
type EnvReader interface {
	GetStringOptional(ctx context.Context, envKey, awsKey, defaultVal string) string
	GetSecretOptional(ctx context.Context, envKey, awsKey, defaultVal string) string
	GetBoolPtr(ctx context.Context, envKey, awsKey string) *bool
}

// EnvSettings reads the AI_* environment into ProviderSettings; the caller attaches Search.
func EnvSettings(ctx context.Context, cfg EnvReader) ProviderSettings {
	return ProviderSettings{
		Provider:   strings.ToLower(strings.TrimSpace(cfg.GetStringOptional(ctx, "AI_PROVIDER", "ai_provider", ""))),
		APIKey:     cfg.GetSecretOptional(ctx, "AI_API_KEY", "ai_api_key", ""),
		BaseURL:    cfg.GetStringOptional(ctx, "AI_BASE_URL", "ai_base_url", ""),
		Model:      cfg.GetStringOptional(ctx, "AI_MODEL", "ai_model", ""),
		ModelTrial: cfg.GetStringOptional(ctx, "AI_MODEL_TRIAL", "ai_model_trial", ""),
		ModelPaid:  cfg.GetStringOptional(ctx, "AI_MODEL_PAID", "ai_model_paid", ""),
		Free:       cfg.GetBoolPtr(ctx, "AI_FREE", "ai_free"),
	}
}

// WithSaved lays the provider saved in Instance settings over the environment.
// A saved provider replaces the environment's key, endpoint and models as a
// whole, never field by field, so an environment key is never sent to a saved
// provider's host. With no saved provider the environment stands. AI_FREE and
// search stay environment-owned.
func (s ProviderSettings) WithSaved(provider, apiKey, baseURL, model string) ProviderSettings {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return s
	}
	s.Provider, s.APIKey, s.BaseURL, s.Model = provider, apiKey, baseURL, model
	s.ModelTrial, s.ModelPaid = "", ""
	return s
}
