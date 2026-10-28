package handler

import (
	"net/http"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func segmentParams(c *gin.Context, withID bool) (orgID uuid.UUID, id uuid.UUID, ok bool) {
	org := middleware.GetOrganizationID(c)
	if org == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return uuid.Nil, uuid.Nil, false
	}
	if withID {
		parsed, err := uuid.Parse(c.Param("id"))
		if err != nil {
			errx.JSON(c, errx.New(errx.BadRequest, "invalid segment id"))
			return uuid.Nil, uuid.Nil, false
		}
		id = parsed
	}
	return *org, id, true
}

func (h *Handler) ListSegments(c *gin.Context) {
	orgID, _, ok := segmentParams(c, false)
	if !ok {
		return
	}
	segs, xerr := h.ContactService.ListSegments(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": segs})
}

func (h *Handler) GetSegment(c *gin.Context) {
	orgID, id, ok := segmentParams(c, true)
	if !ok {
		return
	}
	seg, xerr := h.ContactService.GetSegment(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, seg)
}

func (h *Handler) CreateSegment(c *gin.Context) {
	orgID, _, ok := segmentParams(c, false)
	if !ok {
		return
	}
	var req models.SaveSegmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	var userID *uuid.UUID
	if u, err := middleware.GetUserUUID(c); err == nil {
		userID = &u
	}
	seg, xerr := h.ContactService.CreateSegment(c.Request.Context(), orgID, userID, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntitySegment, &seg.ID, nil, map[string]string{"name": seg.Name})
	c.JSON(http.StatusCreated, seg)
}

func (h *Handler) UpdateSegment(c *gin.Context) {
	orgID, id, ok := segmentParams(c, true)
	if !ok {
		return
	}
	var req models.SaveSegmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	seg, xerr := h.ContactService.UpdateSegment(c.Request.Context(), orgID, id, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionUpdate, models.AuditEntitySegment, &seg.ID, nil, map[string]string{"name": seg.Name})
	c.JSON(http.StatusOK, seg)
}

func (h *Handler) DeleteSegment(c *gin.Context) {
	orgID, id, ok := segmentParams(c, true)
	if !ok {
		return
	}
	if xerr := h.ContactService.DeleteSegment(c.Request.Context(), orgID, id); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySegment, &id, nil, nil)
	c.Status(http.StatusNoContent)
}

func (h *Handler) EnrollSegment(c *gin.Context) {
	orgID, id, ok := segmentParams(c, true)
	if !ok {
		return
	}
	var req models.EnrollSegmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	campaignID, err := uuid.Parse(req.CampaignID)
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid campaign_id"))
		return
	}
	res, xerr := h.ContactService.EnrollSegment(c.Request.Context(), middleware.GetUserID(c), orgID, id, campaignID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionAssign, models.AuditEntitySegment, &id, nil, map[string]string{"campaign_id": campaignID.String()})
	c.JSON(http.StatusOK, res)
}
