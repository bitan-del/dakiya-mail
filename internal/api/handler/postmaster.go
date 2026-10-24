package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/app/postmaster"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
)

// Gmail Postmaster Tools: Gmail's own view of the workspace's sending domains.

func (h *Handler) postmasterReady(c *gin.Context) bool {
	if h.PostmasterService == nil {
		errx.JSON(c, errx.New(errx.NotImplemented, "Google Postmaster is not enabled on this instance"))
		return false
	}
	return true
}

// GetPostmasterReport is GET /deliverability/postmaster?domain=&days=.
func (h *Handler) GetPostmasterReport(c *gin.Context) {
	if !h.postmasterReady(c) {
		return
	}
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.ErrNoOrganization)
		return
	}
	days := postmaster.DefaultDays
	if q := c.Query("days"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil {
			errx.JSON(c, postmaster.ErrInvalidDays)
			return
		}
		days = n
	}
	rep, xerr := h.PostmasterService.Report(c.Request.Context(), *orgID, c.Query("domain"), days)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, rep)
}

// PutPostmasterCredentials is PUT /deliverability/postmaster/credentials. It replaces whatever is stored.
func (h *Handler) PutPostmasterCredentials(c *gin.Context) {
	if !h.postmasterReady(c) {
		return
	}
	var req models.PostmasterCredentials
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	if xerr := h.PostmasterService.Connect(c.Request.Context(), req); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionConnect, models.AuditEntityPostmaster, nil, nil, nil)
	c.Status(http.StatusNoContent)
}

// DeletePostmasterCredentials is DELETE /deliverability/postmaster/credentials. Stored history stays.
func (h *Handler) DeletePostmasterCredentials(c *gin.Context) {
	if !h.postmasterReady(c) {
		return
	}
	if xerr := h.PostmasterService.Disconnect(c.Request.Context()); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDisconnect, models.AuditEntityPostmaster, nil, nil, nil)
	c.Status(http.StatusNoContent)
}
