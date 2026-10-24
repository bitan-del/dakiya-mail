package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequireAPIKeyEmailAccountParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allowedID := uuid.New()
	deniedID := uuid.New()

	tests := []struct {
		name       string
		authType   string
		allowlist  []uuid.UUID
		pathID     uuid.UUID
		wantStatus int
	}{
		{"jwt bypasses allowlist", AuthTypeJWT, []uuid.UUID{allowedID}, deniedID, http.StatusOK},
		{"unrestricted key bypasses allowlist", AuthTypeAPIKey, nil, deniedID, http.StatusOK},
		{"allowed key passes", AuthTypeAPIKey, []uuid.UUID{allowedID}, allowedID, http.StatusOK},
		{"denied key fails", AuthTypeAPIKey, []uuid.UUID{allowedID}, deniedID, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequestIDMiddleware())
			r.GET("/emails/:id",
				func(c *gin.Context) {
					c.Set(AuthTypeKey, tt.authType)
					c.Set(APIKeyAllowedEmailAccountsKey, tt.allowlist)
					c.Next()
				},
				RequireAPIKeyEmailAccountParam("id"),
				func(c *gin.Context) {
					c.Status(http.StatusOK)
				},
			)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/emails/"+tt.pathID.String(), nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestRequireAPIPermission(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		granted    uint64
		required   uint64
		wantStatus int
	}{
		{"jwt-bypass", AuthTypeJWT, 0, models.APIPermReadCampaigns, http.StatusOK},
		{"apikey-granted", AuthTypeAPIKey, models.APIPermReadCampaigns | models.APIPermReadContacts, models.APIPermReadCampaigns, http.StatusOK},
		{"apikey-missing", AuthTypeAPIKey, models.APIPermReadContacts, models.APIPermReadCampaigns, http.StatusForbidden},
		{"apikey-no-perms-key", AuthTypeAPIKey, 0, models.APIPermReadCampaigns, http.StatusForbidden},
		{"unauthenticated", "", 0, models.APIPermReadCampaigns, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tt.authType != "" {
					c.Set(AuthTypeKey, tt.authType)
				}
				if tt.authType == AuthTypeAPIKey && tt.granted != 0 {
					c.Set(APIKeyPermissionsKey, tt.granted)
				}
				c.Next()
			})
			r.GET("/x", RequireAPIPermission(tt.required), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireAccessAPIKeyPath(t *testing.T) {
	// JWT path is exercised in handler-level integration tests; here we
	// pin the API-key branch since it doesn't depend on OrganizationService.
	h := &Handler{}

	tests := []struct {
		name       string
		granted    uint64
		required   uint64
		wantStatus int
	}{
		{"granted", models.APIPermWriteCampaigns | models.APIPermReadAnalytics, models.APIPermWriteCampaigns, http.StatusOK},
		{"missing", models.APIPermReadAnalytics, models.APIPermWriteCampaigns, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(AuthTypeKey, AuthTypeAPIKey)
				c.Set(APIKeyPermissionsKey, tt.granted)
				c.Next()
			})
			r.GET("/x", h.RequireAccess(models.PermManageCampaigns, tt.required), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
