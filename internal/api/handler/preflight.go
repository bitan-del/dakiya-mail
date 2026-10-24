package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
)

// GetCampaignPreflight is GET /campaigns/:id/preflight, the pre-send
// deliverability gate. It measures the campaign as it is now, stores nothing
// and sends nothing, so repeating it is always safe and needs no Idempotency-Key.
func (h *Handler) GetCampaignPreflight(c *gin.Context) {
	if h.PreflightService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "the deliverability preflight is not configured on this server"))
		return
	}
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.ErrNoOrganization)
		return
	}
	campaignID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.ErrUuid)
		return
	}
	report, xerr := h.PreflightService.Run(c.Request.Context(), *orgID, campaignID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, report)
}
