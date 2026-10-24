package handler

import (
	"net/http"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminListOrganizations returns the paginated admin org listing.
func (h *Handler) AdminListOrganizations(c *gin.Context) {
	var search models.AdminOrgSearch
	if err := c.ShouldBindQuery(&search); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid query parameters"))
		return
	}

	result, xerr := h.OrganizationService.SearchOrganizationsForAdmin(c.Request.Context(), &search)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, result)
}

// AdminGetOrganization returns the detail payload for a single org.
func (h *Handler) AdminGetOrganization(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid organization ID"))
		return
	}

	detail, xerr := h.OrganizationService.GetOrganizationAdminDetail(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// AdminGetOrganizationMembers returns the members of an org with users.
func (h *Handler) AdminGetOrganizationMembers(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid organization ID"))
		return
	}

	members, xerr := h.OrganizationService.GetOrganizationMembersForAdmin(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, &models.AdminOrgMembersResult{Data: members})
}

// AdminGetOrgOverrides returns the override row for an org, or 200 with
// null when no admin has touched it yet. Read access requires only
// view_organizations.
func (h *Handler) AdminGetOrgOverrides(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid organization ID"))
		return
	}

	o, xerr := h.OrganizationService.GetLimitOverrides(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, o)
}

// AdminUpdateOrgOverrides upserts the override row. Audit row captures
// only the request body (which is the diff the admin asked for) plus
// the org id, not the post-write state — that's already on the
// override row's granted_by/granted_at/updated_at.
func (h *Handler) AdminUpdateOrgOverrides(c *gin.Context) {
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}

	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid organization ID"))
		return
	}

	var req models.UpdateOrgOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid request body"))
		return
	}
	if !anyOverrideFieldSet(&req) {
		errx.JSON(c, errx.New(errx.BadRequest, "no override fields provided"))
		return
	}

	o, xerr := h.OrganizationService.SetLimitOverrides(c.Request.Context(), orgID, &req, *adminID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}

	h.AdminService.LogAdminAction(
		c.Request.Context(),
		*adminID,
		"set_org_limit_overrides",
		"organization",
		&orgID,
		overrideAuditDetails(&req),
		c.ClientIP(),
		c.Request.UserAgent(),
	)

	c.JSON(http.StatusOK, o)
}

func anyOverrideFieldSet(req *models.UpdateOrgOverridesRequest) bool {
	return req.MaxCampaigns != nil ||
		req.MaxActiveCampaigns != nil ||
		req.MaxTeamMembers != nil ||
		req.MaxEmailAccounts != nil ||
		req.MaxContacts != nil ||
		req.DailyCampaignLimit != nil ||
		req.Notes != nil
}

func overrideAuditDetails(req *models.UpdateOrgOverridesRequest) map[string]any {
	d := map[string]any{}
	if req.MaxCampaigns != nil {
		d["max_campaigns"] = *req.MaxCampaigns
	}
	if req.MaxActiveCampaigns != nil {
		d["max_active_campaigns"] = *req.MaxActiveCampaigns
	}
	if req.MaxTeamMembers != nil {
		d["max_team_members"] = *req.MaxTeamMembers
	}
	if req.MaxEmailAccounts != nil {
		d["max_email_accounts"] = *req.MaxEmailAccounts
	}
	if req.MaxContacts != nil {
		d["max_contacts"] = *req.MaxContacts
	}
	if req.DailyCampaignLimit != nil {
		d["daily_campaign_limit"] = *req.DailyCampaignLimit
	}
	if req.Notes != nil {
		d["notes"] = *req.Notes
	}
	return d
}
