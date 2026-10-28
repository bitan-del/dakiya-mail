package handler

import (
	"fmt"
	"html"
	"net/http"
	"net/mail"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/app/emailsend"
	"github.com/bitan-del/dakiya-mail/internal/app/template"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxTransactionalBodyBytes = 1 << 20

// TransactionalSend sends one email to one recipient now (or after delay_seconds),
// from a template or a body, and returns the Message-ID it will carry.
func (h *Handler) TransactionalSend(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	userID, err := middleware.GetUserUUID(c)
	if err != nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	var req models.TransactionalSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	bad := func(field, msg string) {
		errx.JSON(c, errx.NewWithIdentifier(errx.BadRequest, "invalid_"+field, msg))
	}
	ctx := c.Request.Context()

	addr, perr := mail.ParseAddress(strings.TrimSpace(req.To.Email))
	if perr != nil {
		bad("to", "to.email must be one valid address")
		return
	}
	to := (&mail.Address{Name: strings.TrimSpace(req.To.Name), Address: strings.ToLower(addr.Address)}).String()
	if req.DelaySeconds < 0 || req.DelaySeconds > models.MaxTransactionalDelaySeconds {
		bad("delay_seconds", fmt.Sprintf("delay_seconds must be between 0 and %d", models.MaxTransactionalDelaySeconds))
		return
	}
	if len(req.TrackIDs) > models.MaxTransactionalTrackIDs {
		bad("track_ids", "at most 5 track_ids")
		return
	}
	for _, id := range req.TrackIDs {
		if len(id) > models.MaxTransactionalTrackIDLen {
			bad("track_ids", "each track_id is at most 255 characters")
			return
		}
	}
	if len(req.HTMLBody) > maxTransactionalBodyBytes || len(req.TextBody) > maxTransactionalBodyBytes {
		bad("body", "html_body and text_body are each at most 1 MB")
		return
	}

	// Substitution values are escaped where they land in HTML.
	htmlVars := make(map[string]string, len(req.Substitutions))
	for k, v := range req.Substitutions {
		htmlVars[k] = html.EscapeString(v)
	}
	subject := template.RenderString(req.Subject, req.Substitutions)
	bodyHTML := template.RenderString(req.HTMLBody, htmlVars)
	bodyPlain := template.RenderString(req.TextBody, req.Substitutions)
	var templateID *uuid.UUID
	if raw := strings.TrimSpace(req.TemplateID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			bad("template_id", "template_id is not a valid id")
			return
		}
		rendered, xerr := h.TemplateService.Render(ctx, *orgID, id, req.Substitutions)
		if xerr != nil {
			errx.JSON(c, xerr)
			return
		}
		htmlRendered, _ := h.TemplateService.Render(ctx, *orgID, id, htmlVars)
		templateID = &id
		if subject == "" {
			subject = rendered.Subject
		}
		if bodyHTML == "" && htmlRendered != nil {
			bodyHTML = htmlRendered.BodyHTML
		}
		if bodyPlain == "" {
			bodyPlain = rendered.BodyPlain
		}
	}
	if strings.TrimSpace(subject) == "" {
		bad("subject", "subject is required (directly or from the template)")
		return
	}
	if strings.TrimSpace(bodyHTML) == "" && strings.TrimSpace(bodyPlain) == "" {
		bad("body", "one of html_body, text_body or template_id is required")
		return
	}

	if suppressed, reason, xerr := h.AdvancedService.ShouldSuppressRecipient(ctx, *orgID, addr.Address); xerr != nil {
		errx.JSON(c, xerr)
		return
	} else if suppressed {
		errx.JSON(c, errx.NewWithIdentifier(errx.BadRequest, "recipient_suppressed", fmt.Sprintf("%s is suppressed for this workspace (%s)", addr.Address, reason)))
		return
	}

	var accountID *uuid.UUID
	if v := strings.TrimSpace(req.EmailAccountID); v != "" && v != "auto" {
		id, err := uuid.Parse(v)
		if err != nil {
			bad("email_account_id", "email_account_id is not a valid id")
			return
		}
		accountID = &id
	}
	candidate, _, xerr := h.ComposeService.Resolve(ctx, userID, *orgID, accountID, nil, addr.Address)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}

	domain := "dakiya.local"
	if at := strings.LastIndex(candidate.Account.Email, "@"); at >= 0 {
		domain = candidate.Account.Email[at+1:]
	}
	messageID := fmt.Sprintf("<%s@%s>", uuid.NewString(), domain)

	resp, xerr := h.EmailSendService.SendEmail(ctx, userID, *orgID, candidate.Account.ID, &emailsend.SendEmailRequest{
		To:           []string{to},
		Subject:      subject,
		BodyHTML:     bodyHTML,
		BodyPlain:    bodyPlain,
		SendMode:     "instant",
		MessageID:    messageID,
		NoUndoDelay:  true,
		DelaySeconds: req.DelaySeconds,
	})
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	if xerr := h.AdvancedService.RecordTransactionalMessage(ctx, *orgID, resp.TaskID, templateID, strings.ToLower(addr.Address), req.TrackIDs); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionSend, models.AuditEntityUnibox, &resp.TaskID, nil, map[string]string{"kind": "transactional"})

	trackIDs := req.TrackIDs
	if trackIDs == nil {
		trackIDs = []string{}
	}
	c.JSON(http.StatusOK, models.TransactionalSendResponse{
		TaskID:       resp.TaskID,
		MessageID:    messageID,
		AccountEmail: candidate.Account.Email,
		ScheduledAt:  resp.ScheduledAt,
		TrackIDs:     trackIDs,
	})
}

func (h *Handler) GetTransactionalMessage(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	taskID, err := uuid.Parse(c.Param("task_id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid task_id"))
		return
	}
	m, xerr := h.AdvancedService.GetTransactionalMessage(c.Request.Context(), *orgID, taskID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, m)
}
