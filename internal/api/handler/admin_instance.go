package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/app/instancecheck"
	"github.com/bitan-del/dakiya-mail/internal/app/instanceconfig"
	"github.com/bitan-del/dakiya-mail/internal/app/instancesettings"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/pkg/generation"
	"github.com/gin-gonic/gin"
)

// The Instance surface answers the two questions a self-hoster cannot answer
// from inside the product today: what did my environment actually resolve to,
// and what is wrong with this deployment right now.

// AdminInstanceConfig returns the resolved effective configuration.
//
// Read-only by design: the environment is authoritative, so no API ever writes
// a key on this page. Sensitive values never leave the process; they carry a
// short fingerprint instead, which is enough to confirm two services hold the
// same AUTH_SECRET without disclosing either.
func (h *Handler) AdminInstanceConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"entries": instanceconfig.Entries(h.InstanceRuntime)})
}

// AdminInstanceLimits returns the effective product limits. Every value is a
// compiled constant, so this exists for visibility, not for editing.
func (h *Handler) AdminInstanceLimits(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"groups": instanceconfig.Limits()})
}

// AdminInstanceHealth runs the setup and health checks and returns only the
// findings. Checks run in parallel with a per-check timeout, so this is safe
// to poll, and a check whose input is unavailable is skipped rather than
// reported as a failure.
func (h *Handler) AdminInstanceHealth(c *gin.Context) {
	registry := h.InstanceChecks
	if registry == nil {
		// Still answer with the environment-only checks rather than an empty
		// page: a page that says nothing is indistinguishable from a healthy one.
		registry = instancecheck.New(instancecheck.Deps{
			Runtime:   h.InstanceRuntime,
			Transport: h.MailTransportRef,
		})
	}

	checks, summary := registry.Run(c.Request.Context(), instancecheck.Input{
		Host:      c.Request.Host,
		Origin:    c.GetHeader("Origin"),
		Forwarded: c.GetHeader("X-Forwarded-For") != "",
	})

	c.JSON(http.StatusOK, gin.H{"checks": checks, "summary": summary})
}

// AdminGetInstanceSettings returns the database-backed settings document.
func (h *Handler) AdminGetInstanceSettings(c *gin.Context) {
	if h.InstanceSettings == nil {
		c.JSON(http.StatusOK, instancesettings.Defaults())
		return
	}
	c.JSON(http.StatusOK, h.InstanceSettings.Get(c.Request.Context()))
}

// AdminPutInstanceSettings validates, clamps and stores the settings document.
// Absent fields keep their stored value, so a client that does not know about
// a key cannot clear it.
func (h *Handler) AdminPutInstanceSettings(c *gin.Context) {
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	if h.InstanceSettings == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "Instance settings are not available on this deployment."))
		return
	}

	var patch instancesettings.Patch
	if err := c.ShouldBindJSON(&patch); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid request body"))
		return
	}

	doc, err := h.InstanceSettings.Put(c.Request.Context(), patch, adminID)
	if err != nil {
		var invalid *instancesettings.InvalidError
		if errors.As(err, &invalid) {
			errx.JSON(c, errx.New(errx.BadRequest, invalid.Msg))
			return
		}
		errx.JSON(c, errx.InternalError())
		return
	}

	if h.AdminService != nil {
		h.AdminService.LogAdminAction(
			c.Request.Context(),
			*adminID,
			"update_instance_settings",
			"instance",
			nil,
			instanceSettingsAuditDetails(doc, patch.AI.ChangesKey()),
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}

	c.JSON(http.StatusOK, doc)
}

func instanceSettingsAuditDetails(doc instancesettings.Document, aiKeyChanged bool) map[string]any {
	details := map[string]any{
		"invitations_links_enabled":          doc.Invitations.LinksEnabled,
		"invitations_ttl_hours":              doc.Invitations.TTLHours,
		"access_allow_invited_signup":        doc.Access.AllowInvitedSignup,
		"sync_backfill_days":                 doc.Sync.BackfillDays,
		"sync_backfill_messages":             doc.Sync.BackfillMessages,
		"sync_daily_messages_mailbox":        doc.Sync.DailyMessagesPerMailbox,
		"sync_daily_messages_org":            doc.Sync.DailyMessagesPerOrg,
		"deliverability_enforce_domain_auth": doc.Deliverability.EnforceDomainAuth,
		"deliverability_auth_grace_hours":    doc.Deliverability.AuthGraceHours,
		"ai_provider":                        doc.AI.Provider,
		"ai_model":                           doc.AI.Model,
		"ai_base_url":                        doc.AI.BaseURL,
		"ai_api_key_set":                     doc.AI.APIKeySet,
		"ai_api_key_changed":                 aiKeyChanged,
	}
	return details
}

// aiTestRequest is the form as it stands, so a provider can be checked before it is saved.
type aiTestRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

// AdminTestInstanceAI makes one tiny completion and answers ok or the provider's own error.
// A body with a provider tests that draft (an empty key reuses the saved one for the same
// provider); an empty body tests what the instance resolves to now, saved settings over env.
// The key never appears in the response.
func (h *Handler) AdminTestInstanceAI(c *gin.Context) {
	var req aiTestRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid request body"))
		return
	}
	ctx := c.Request.Context()

	var saved instancesettings.AI
	if h.InstanceSettings != nil {
		saved = h.InstanceSettings.AI(ctx)
	}
	settings := h.AIEnv.WithSaved(saved.Provider, saved.APIKey, saved.BaseURL, saved.Model)
	if provider := strings.ToLower(strings.TrimSpace(req.Provider)); provider != "" {
		key := strings.TrimSpace(req.APIKey)
		if key == "" && provider == saved.Provider {
			key = saved.APIKey
		}
		settings = generation.ProviderSettings{
			Provider: provider,
			APIKey:   key,
			BaseURL:  strings.TrimSpace(req.BaseURL),
			Model:    strings.TrimSpace(req.Model),
			Free:     settings.Free,
		}
	}
	c.JSON(http.StatusOK, testAIProvider(ctx, settings))
}

func testAIProvider(ctx context.Context, s generation.ProviderSettings) gin.H {
	fail := func(msg string) gin.H { return gin.H{"ok": false, "error": scrubSecret(msg, s.APIKey)} }

	cfg, err := generation.Resolve(s)
	if err != nil {
		return fail(err.Error())
	}
	provider, err := generation.NewProvider(cfg)
	if errors.Is(err, generation.ErrProviderNotConfigured) {
		if s.Provider == "" {
			return fail("No AI provider is configured on this instance.")
		}
		return fail("This provider needs an API key.")
	}
	if err != nil {
		return fail(err.Error())
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	// Generous cap: a reasoning model spends hidden tokens before it answers.
	res, err := provider.Complete(ctx, generation.CompletionRequest{
		System:    "You are a connection check. Answer in one word.",
		Prompt:    "Reply with: ok",
		MaxTokens: 256,
	})
	if err != nil {
		return fail(err.Error())
	}
	return gin.H{"ok": true, "provider": provider.Name(), "model": res.Model, "latency_ms": time.Since(start).Milliseconds()}
}

// scrubSecret keeps a provider's error message from echoing the key back and bounds its length.
func scrubSecret(msg, secret string) string {
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	if r := []rune(msg); len(r) > 400 {
		msg = string(r[:400]) + "..."
	}
	return msg
}
