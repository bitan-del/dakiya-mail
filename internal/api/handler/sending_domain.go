package handler

import (
	"net/http"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func domainParams(c *gin.Context, withID bool) (uuid.UUID, uuid.UUID, bool) {
	org := middleware.GetOrganizationID(c)
	if org == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return uuid.Nil, uuid.Nil, false
	}
	var id uuid.UUID
	if withID {
		parsed, err := uuid.Parse(c.Param("id"))
		if err != nil {
			errx.JSON(c, errx.New(errx.BadRequest, "invalid id"))
			return uuid.Nil, uuid.Nil, false
		}
		id = parsed
	}
	return *org, id, true
}

func (h *Handler) ListSendingDomains(c *gin.Context) {
	orgID, _, ok := domainParams(c, false)
	if !ok {
		return
	}
	list, xerr := h.EmailService.ListSendingDomains(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) GetSendingDomain(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	d, xerr := h.EmailService.GetSendingDomain(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) AddSendingDomain(c *gin.Context) {
	orgID, _, ok := domainParams(c, false)
	if !ok {
		return
	}
	var req models.SaveSendingDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	d, xerr := h.EmailService.AddSendingDomain(c.Request.Context(), orgID, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntitySendingDomain, &d.ID, nil, map[string]string{"domain": d.Domain})
	c.JSON(http.StatusCreated, d)
}

func (h *Handler) UpdateSendingDomain(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	var req models.SaveSendingDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	d, xerr := h.EmailService.UpdateSendingDomain(c.Request.Context(), orgID, id, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionUpdate, models.AuditEntitySendingDomain, &d.ID, nil, map[string]string{"domain": d.Domain})
	c.JSON(http.StatusOK, d)
}

func (h *Handler) VerifySendingDomain(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	d, xerr := h.EmailService.VerifySendingDomain(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionTest, models.AuditEntitySendingDomain, &d.ID, nil, map[string]string{"domain": d.Domain, "state": d.State})
	c.JSON(http.StatusOK, d)
}

func (h *Handler) DeleteSendingDomain(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	if xerr := h.EmailService.DeleteSendingDomain(c.Request.Context(), orgID, id); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySendingDomain, &id, nil, nil)
	c.Status(http.StatusNoContent)
}
