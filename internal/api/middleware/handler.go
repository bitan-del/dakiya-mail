package middleware

import (
	"github.com/bitan-del/dakiya-mail/internal/app/apikey"
	"github.com/bitan-del/dakiya-mail/internal/app/idempotency"
	"github.com/bitan-del/dakiya-mail/internal/app/oauth"
	"github.com/bitan-del/dakiya-mail/internal/app/organization"
	"github.com/bitan-del/dakiya-mail/internal/app/poollink"
	"github.com/bitan-del/dakiya-mail/internal/app/ratelimit"
	"github.com/bitan-del/dakiya-mail/internal/app/token"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cache"
)

type Handler struct {
	TokenService        token.TokenService
	APIKeyService       apikey.APIKeyService
	IdempotencyService  idempotency.Service
	RateLimitService    ratelimit.RateLimitService
	OrganizationService organization.OrganizationService
	// OAuthService validates OAuth 2.1 bearer access tokens (nil-safe: when
	// unset, only JWT + API-key auth are accepted).
	OAuthService *oauth.Service
	// Cache backs the pre-login per-IP limiter, which cannot use
	// RateLimitService because that one is keyed on a user id that does not
	// exist yet.
	Cache *cache.Cache
	// PoolLinkService authenticates linked instances; nil-safe, routes answer 501.
	PoolLinkService poollink.Service
}
