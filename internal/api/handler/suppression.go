package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/app/advanced"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/utils/paging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// maxSuppressionUploadBytes fits about 1.5M SHA-256 digests or 2M addresses.
const maxSuppressionUploadBytes = 100 * 1024 * 1024

func (h *Handler) ListSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	var cursor *uuid.UUID
	if raw := c.Query("cursor"); raw != "" {
		id, err := paging.DecodeUUID(raw)
		if err != nil {
			errx.JSON(c, errx.New(errx.BadRequest, "invalid cursor"))
			return
		}
		cursor = &id
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		l, err := strconv.Atoi(raw)
		if err != nil || l < 1 || l > 200 {
			errx.JSON(c, errx.New(errx.BadRequest, "limit must be between 1 and 200"))
			return
		}
		limit = l
	}
	res, xerr := h.AdvancedService.ListSuppressions(c.Request.Context(), *orgID, c.Query("q"), c.Query("source"), limit, cursor)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) AddSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	var req models.AddSuppressionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	res, xerr := h.AdvancedService.AddSuppressions(c.Request.Context(), *orgID, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntitySuppression, nil, nil, map[string]string{"added": strconv.Itoa(res.Added)})
	c.JSON(http.StatusOK, res)
}

func (h *Handler) RemoveSuppression(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid id"))
		return
	}
	if xerr := h.AdvancedService.RemoveSuppression(c.Request.Context(), *orgID, id); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySuppression, &id, nil, nil)
	c.Status(http.StatusNoContent)
}

// RemoveSuppressionEmails is a POST so the address list stays out of the URL.
func (h *Handler) RemoveSuppressionEmails(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	var req models.RemoveSuppressionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	n, xerr := h.AdvancedService.RemoveSuppressionEmails(c.Request.Context(), *orgID, req.Emails)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySuppression, nil, nil, map[string]string{"removed": strconv.Itoa(n)})
	c.JSON(http.StatusOK, gin.H{"removed": n})
}

func (h *Handler) ImportSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSuppressionUploadBytes)
	fh, err := c.FormFile("file")
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "file is required (max 100 MB)"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "could not read the uploaded file"))
		return
	}
	defer f.Close()

	format := advanced.SuppressionFormat(strings.ToLower(strings.TrimSpace(c.PostForm("format"))))
	if format == "" {
		format = advanced.SuppressionFormatPlain
	}
	listName := strings.TrimSpace(c.PostForm("list_name"))
	if listName == "" && format != advanced.SuppressionFormatPlain {
		listName = strings.TrimSuffix(fh.Filename, ".csv")
	}
	res, xerr := h.AdvancedService.ImportSuppressions(c.Request.Context(), *orgID, f, format, listName, c.PostForm("reason"))
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionImport, models.AuditEntitySuppression, nil, nil, map[string]string{
		"format":    string(format),
		"list_name": listName,
		"added":     strconv.Itoa(res.Added),
	})
	c.JSON(http.StatusOK, res)
}

func (h *Handler) ExportSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	format := advanced.SuppressionFormat(strings.ToLower(c.DefaultQuery("format", "plain")))
	var buf bytes.Buffer
	rows, xerr := h.AdvancedService.ExportSuppressions(c.Request.Context(), *orgID, &buf, format)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionExport, models.AuditEntitySuppression, nil, nil, map[string]string{"format": string(format), "rows": strconv.Itoa(rows)})
	name := fmt.Sprintf("suppression-%s-%s.csv", format, time.Now().UTC().Format("2006-01-02"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("X-Total-Rows", strconv.Itoa(rows))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (h *Handler) ListSuppressionHashLists(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	lists, xerr := h.AdvancedService.ListSuppressionHashLists(c.Request.Context(), *orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": lists})
}

func (h *Handler) DeleteSuppressionHashList(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	var req struct {
		Algo     models.SuppressionHashAlgo `json:"algo"`
		ListName string                     `json:"list_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	n, xerr := h.AdvancedService.DeleteSuppressionHashList(c.Request.Context(), *orgID, req.Algo, req.ListName)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySuppression, nil, nil, map[string]string{"list_name": req.ListName, "algo": string(req.Algo)})
	c.JSON(http.StatusOK, gin.H{"removed": n})
}
