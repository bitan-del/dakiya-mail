package handler

import (
	"net/http"
	"strconv"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListBrands(c *gin.Context) {
	orgID, _, ok := domainParams(c, false)
	if !ok {
		return
	}
	list, xerr := h.EmailService.ListBrands(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) GetBrand(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	b, xerr := h.EmailService.GetBrand(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, b)
}

func (h *Handler) saveBrand(c *gin.Context, create bool) {
	orgID, id, ok := domainParams(c, !create)
	if !ok {
		return
	}
	var req models.SaveBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	target := &id
	action, status := models.AuditActionUpdate, http.StatusOK
	if create {
		target, action, status = nil, models.AuditActionCreate, http.StatusCreated
	}
	b, xerr := h.EmailService.SaveBrand(c.Request.Context(), orgID, target, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, action, models.AuditEntityBrand, &b.ID, nil, map[string]string{"name": b.Name})
	c.JSON(status, b)
}

func (h *Handler) CreateBrand(c *gin.Context) { h.saveBrand(c, true) }
func (h *Handler) UpdateBrand(c *gin.Context) { h.saveBrand(c, false) }

func (h *Handler) DeleteBrand(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	if xerr := h.EmailService.DeleteBrand(c.Request.Context(), orgID, id); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntityBrand, &id, nil, nil)
	c.Status(http.StatusNoContent)
}

func (h *Handler) SetBrandMailboxes(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	var req models.SetBrandMailboxesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	b, xerr := h.EmailService.SetBrandMailboxes(c.Request.Context(), orgID, id, req.EmailAccountIDs)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionAssign, models.AuditEntityBrand, &b.ID, nil, map[string]string{"mailboxes": strconv.Itoa(len(b.MailboxIDs))})
	c.JSON(http.StatusOK, b)
}
