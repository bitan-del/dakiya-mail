package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
)

// SalesBlink warmup: Settings > SalesBlink.

func (h *Handler) salesBlinkReady(c *gin.Context) bool {
	if h.SalesBlinkService == nil {
		errx.JSON(c, errx.New(errx.NotImplemented, "salesblink warmup is not enabled on this instance"))
		return false
	}
	return true
}

func salesBlinkOrg(c *gin.Context) (uuid.UUID, bool) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.ErrNoOrganization)
		return uuid.Nil, false
	}
	return *orgID, true
}

func (h *Handler) SalesBlinkStatus(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	orgID, ok := salesBlinkOrg(c)
	if !ok {
		return
	}
	st, xerr := h.SalesBlinkService.Status(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) SalesBlinkConnect(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	userID, err := middleware.GetUserUUID(c)
	if err != nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	var req struct {
		APIKey     string `json:"api_key"`
		BaseURL    string `json:"base_url"`
		AutoEnroll *bool  `json:"auto_enroll"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	autoEnroll := true
	if req.AutoEnroll != nil {
		autoEnroll = *req.AutoEnroll
	}
	l, xerr := h.SalesBlinkService.Connect(c.Request.Context(), userID, req.APIKey, req.BaseURL, autoEnroll)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusCreated, l)
}

func (h *Handler) SalesBlinkUpdateSettings(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	var req struct {
		AutoEnroll        bool `json:"auto_enroll"`
		WarmupPerDay      int  `json:"warmup_per_day"`
		MaxDailyFrequency int  `json:"max_daily_frequency"`
		RampUpFrequency   int  `json:"ramp_up_frequency"`
		AutoRampUp        bool `json:"auto_ramp_up"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	l, xerr := h.SalesBlinkService.UpdateSettings(c.Request.Context(), req.AutoEnroll,
		req.WarmupPerDay, req.MaxDailyFrequency, req.RampUpFrequency, req.AutoRampUp)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, l)
}

func (h *Handler) SalesBlinkDisconnect(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	if xerr := h.SalesBlinkService.Disconnect(c.Request.Context()); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) SalesBlinkMailboxes(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	orgID, ok := salesBlinkOrg(c)
	if !ok {
		return
	}
	rows, xerr := h.SalesBlinkService.ListMailboxes(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// SalesBlinkSync enrolls every eligible mailbox that is not already enrolled.
func (h *Handler) SalesBlinkSync(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	orgID, ok := salesBlinkOrg(c)
	if !ok {
		return
	}
	res, xerr := h.SalesBlinkService.SyncAll(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, res)
}

func salesBlinkAccountID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	orgID, ok := salesBlinkOrg(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.ErrUuid)
		return uuid.Nil, uuid.Nil, false
	}
	return orgID, id, true
}

func (h *Handler) SalesBlinkEnroll(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	orgID, accountID, ok := salesBlinkAccountID(c)
	if !ok {
		return
	}
	row, xerr := h.SalesBlinkService.Enroll(c.Request.Context(), orgID, accountID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *Handler) SalesBlinkUnenroll(c *gin.Context) {
	if !h.salesBlinkReady(c) {
		return
	}
	orgID, accountID, ok := salesBlinkAccountID(c)
	if !ok {
		return
	}
	if xerr := h.SalesBlinkService.Unenroll(c.Request.Context(), orgID, accountID); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.Status(http.StatusNoContent)
}
