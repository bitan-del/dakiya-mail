package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
)

// OnboardingOAuthStartRequest starts an OAuth round trip for a Gmail or Outlook account.
type OnboardingOAuthStartRequest struct {
	Provider string `json:"provider"`
}

// OnboardingOAuthFinishRequest carries the authorization code + state back from the provider.
type OnboardingOAuthFinishRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// OnboardingSMTPIMAPRequest connects an SMTP/IMAP mailbox in a single call.
type OnboardingSMTPIMAPRequest struct {
	Email string          `json:"email"`
	Name  string          `json:"name"`
	SMTP  *models.Service `json:"smtp"`
	IMAP  *models.Service `json:"imap"`
}

func (h *Handler) StartEmailOAuth(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.ErrNoOrganization)
		return
	}

	var req OnboardingOAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	resp, xerr := h.EmailService.OAuthStart(c.Request.Context(), userID, orgID, models.InboxProvider(req.Provider))
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) FinishEmailOAuth(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)

	var req OnboardingOAuthFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	acc, xerr := h.EmailService.OAuthFinish(c.Request.Context(), userIDStr, req.Code, req.State)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionConnect, models.AuditEntityEmailAccount, &acc.ID, nil, map[string]string{
		"provider": acc.Provider,
		"email":    acc.Email,
	})

	c.JSON(http.StatusCreated, acc)
}

func (h *Handler) ConnectEmailSMTPIMAP(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.ErrNoOrganization)
		return
	}

	var req OnboardingSMTPIMAPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	acc, xerr := h.EmailService.OnboardSMTPIMAP(c.Request.Context(), userIDStr, orgID, &models.NewSMTPIMAPAccount{
		Email: req.Email,
		Name:  req.Name,
		SMTP:  req.SMTP,
		IMAP:  req.IMAP,
	})
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionConnect, models.AuditEntityEmailAccount, &acc.ID, nil, map[string]string{
		"provider": "smtp_imap",
		"email":    acc.Email,
	})

	// Hand the new mailbox to SalesBlink warmup if that is switched on. Done
	// after the audit entry and off the response path: enrolling is a remote
	// call, and a warmup failure must not fail the mailbox connection. The
	// request context is already finished by then, so use a fresh one.
	if h.SalesBlinkService != nil {
		accountID := acc.ID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			h.SalesBlinkService.AutoEnroll(ctx, accountID)
		}()
	}

	c.JSON(http.StatusCreated, acc)
}
