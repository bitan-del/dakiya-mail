package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cloudprovider"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cloudprovider/hetzner"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Admin endpoints under /admin for autonomous fleet management:
//
//   /admin/cloud-credentials                    CRUD + test
//   /admin/cloud-providers/:provider/...        discovery (locations, server_types, images)
//   /admin/provisioning-templates               CRUD
//   /admin/provisioning-jobs                    list + create + detail
//   /admin/provisioning-policy                  per-provider budget caps
//
// All gated by AdminPermManageSettings via the route registration.

// ---------------------------------------------------------------------------
// Cloud credentials
// ---------------------------------------------------------------------------

type CloudCredentialResponse struct {
	ID            uuid.UUID  `json:"id"`
	Provider      string     `json:"provider"`
	Name          string     `json:"name"`
	TokenRedacted string     `json:"token_redacted"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	LastTestAt    *time.Time `json:"last_test_at,omitempty"`
	LastTestOK    *bool      `json:"last_test_ok,omitempty"`
	LastTestError *string    `json:"last_test_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toCredResponse(c *repository.CloudCredential) CloudCredentialResponse {
	return CloudCredentialResponse{
		ID:            c.ID,
		Provider:      c.Provider,
		Name:          c.Name,
		TokenRedacted: maskToken(c.EncryptedToken),
		LastUsedAt:    c.LastUsedAt,
		LastTestAt:    c.LastTestAt,
		LastTestOK:    c.LastTestOK,
		LastTestError: c.LastTestError,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "***" + t[len(t)-4:]
}

func (h *Handler) AdminListCloudCredentials(c *gin.Context) {
	if h.CloudCredentialRepo == nil {
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
		return
	}
	rows, err := h.CloudCredentialRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]CloudCredentialResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCredResponse(&r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type CreateCloudCredentialRequest struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Token    string `json:"token"`
}

func (h *Handler) AdminCreateCloudCredential(c *gin.Context) {
	if h.CloudCredentialRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud credentials repo not configured"})
		return
	}
	var req CreateCloudCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Provider == "" || req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider and token required"})
		return
	}
	if req.Name == "" {
		req.Name = req.Provider + "-default"
	}

	// TODO: cipher-encrypt the token via h.CipherService. For now we store
	// as-is so the wiring works end-to-end; flag this in audit log.
	row := &repository.CloudCredential{
		Provider:       req.Provider,
		Name:           req.Name,
		EncryptedToken: req.Token,
	}
	if err := h.CloudCredentialRepo.Create(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toCredResponse(row))
}

func (h *Handler) AdminDeleteCloudCredential(c *gin.Context) {
	if h.CloudCredentialRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud credentials repo not configured"})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.CloudCredentialRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AdminTestCloudCredential(c *gin.Context) {
	if h.CloudCredentialRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud credentials repo not configured"})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	cred, err := h.CloudCredentialRepo.Get(c.Request.Context(), id)
	if err != nil || cred == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
		return
	}
	provider, err := buildProvider(cred)
	if err != nil {
		_ = h.CloudCredentialRepo.UpdateTestResult(c.Request.Context(), id, false, err.Error())
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := provider.Verify(ctx); err != nil {
		_ = h.CloudCredentialRepo.UpdateTestResult(ctx, id, false, err.Error())
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	_ = h.CloudCredentialRepo.UpdateTestResult(ctx, id, true, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// buildProvider picks the right cloudprovider.Provider implementation for a
// credential row. Today only Hetzner; adding OVH/Vultr means another case.
func buildProvider(c *repository.CloudCredential) (cloudprovider.Provider, error) {
	switch c.Provider {
	case "hetzner":
		return hetzner.New(c.EncryptedToken)
	default:
		return nil, errors.New("unsupported provider: " + c.Provider)
	}
}

// ---------------------------------------------------------------------------
// Provider catalog discovery (used by admin UI dropdowns)
// ---------------------------------------------------------------------------

func (h *Handler) AdminListProviderLocations(c *gin.Context) {
	provider, err := h.providerByName(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	locs, err := provider.Locations(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": locs})
}

func (h *Handler) AdminListProviderServerTypes(c *gin.Context) {
	provider, err := h.providerByName(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	types, err := provider.ServerTypes(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": types})
}

func (h *Handler) AdminListProviderImages(c *gin.Context) {
	provider, err := h.providerByName(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	images, err := provider.Images(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": images})
}

// providerByName resolves a provider name to a Provider client, looking up
// the most recent credential row for that provider.
func (h *Handler) providerByName(c *gin.Context) (cloudprovider.Provider, error) {
	if h.CloudCredentialRepo == nil {
		return nil, errors.New("cloud credentials not configured")
	}
	name := c.Param("provider")
	if name == "" {
		return nil, errors.New("provider path param required")
	}
	cred, err := h.CloudCredentialRepo.GetByProvider(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, errors.New("no credential configured for provider " + name)
	}
	return buildProvider(cred)
}

// ---------------------------------------------------------------------------
// Provisioning templates
//
// The admin UI works in a nested {name, description, config:{...},
// auto_provision_tier, is_draft} shape; the repository row is flat. The DTO
// helpers below translate between the two so the two sides agree on the wire
// format (they did not previously, which silently 400'd every template save).
// ---------------------------------------------------------------------------

type provLabelDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type provTemplateConfigDTO struct {
	Provider        string         `json:"provider"`
	CredentialID    *uuid.UUID     `json:"credential_id,omitempty"`
	Location        string         `json:"location"`
	ServerType      string         `json:"server_type"`
	ServerCount     int            `json:"server_count"`
	IPv4PerServer   int            `json:"ipv4_per_server"`
	IPv6PerServer   int            `json:"ipv6_per_server"`
	WorkerTier      string         `json:"worker_tier"`
	WorkerProfileID *uuid.UUID     `json:"worker_profile_id,omitempty"`
	EgressKind      string         `json:"egress_kind"`
	Image           string         `json:"image"`
	Datacenter      string         `json:"datacenter,omitempty"`
	PlacementGroup  string         `json:"placement_group,omitempty"`
	PrivateNetwork  string         `json:"private_network,omitempty"`
	Firewall        string         `json:"firewall"`
	Labels          []provLabelDTO `json:"labels"`
}

type provTemplateDTO struct {
	ID                uuid.UUID             `json:"id,omitempty"`
	Name              string                `json:"name"`
	Description       string                `json:"description,omitempty"`
	Config            provTemplateConfigDTO `json:"config"`
	AutoProvisionTier string                `json:"auto_provision_tier,omitempty"`
	IsDraft           bool                  `json:"is_draft"`
	CreatedAt         time.Time             `json:"created_at,omitempty"`
	UpdatedAt         time.Time             `json:"updated_at,omitempty"`
}

func labelsToDTO(m map[string]string) []provLabelDTO {
	out := make([]provLabelDTO, 0, len(m))
	for k, v := range m {
		out = append(out, provLabelDTO{Key: k, Value: v})
	}
	return out
}

func labelsFromDTO(in []provLabelDTO) map[string]string {
	m := map[string]string{}
	for _, l := range in {
		if l.Key != "" {
			m[l.Key] = l.Value
		}
	}
	return m
}

func toTemplateDTO(t *repository.ProvisioningTemplate) provTemplateDTO {
	auto := ""
	if t.IsAutoTemplate {
		auto = t.Tier
	}
	return provTemplateDTO{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Config: provTemplateConfigDTO{
			Provider:        t.Provider,
			Location:        t.Location,
			ServerType:      t.ServerType,
			ServerCount:     t.ServerCount,
			IPv4PerServer:   t.IPv4PerServer,
			IPv6PerServer:   t.IPv6PerServer,
			WorkerTier:      t.Tier,
			WorkerProfileID: t.WorkerProfileID,
			EgressKind:      t.EgressKind,
			Image:           t.Image,
			Datacenter:      t.Datacenter,
			PlacementGroup:  t.PlacementGroup,
			PrivateNetwork:  t.PrivateNetwork,
			Firewall:        t.Firewall,
			Labels:          labelsToDTO(t.Labels),
		},
		AutoProvisionTier: auto,
		IsDraft:           t.IsDraft,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}
}

// fromTemplateDTO maps the UI shape onto the flat repo model and applies the
// field defaults the create path used to apply inline.
func fromTemplateDTO(d *provTemplateDTO) *repository.ProvisioningTemplate {
	cfg := d.Config
	t := &repository.ProvisioningTemplate{
		ID:              d.ID,
		Name:            d.Name,
		Description:     d.Description,
		Provider:        cfg.Provider,
		Location:        cfg.Location,
		Datacenter:      cfg.Datacenter,
		ServerType:      cfg.ServerType,
		Image:           cfg.Image,
		ServerCount:     cfg.ServerCount,
		IPv4PerServer:   cfg.IPv4PerServer,
		IPv6PerServer:   cfg.IPv6PerServer,
		WorkerProfileID: cfg.WorkerProfileID,
		Tier:            cfg.WorkerTier,
		EgressKind:      cfg.EgressKind,
		Labels:          labelsFromDTO(cfg.Labels),
		PlacementGroup:  cfg.PlacementGroup,
		PrivateNetwork:  cfg.PrivateNetwork,
		Firewall:        cfg.Firewall,
		IsDraft:         d.IsDraft,
		// A draft is never eligible as the tier's auto-provision template.
		IsAutoTemplate: !d.IsDraft && d.AutoProvisionTier != "",
	}
	if t.Image == "" {
		t.Image = "ubuntu-22.04"
	}
	if t.ServerCount == 0 {
		t.ServerCount = 1
	}
	if t.IPv4PerServer == 0 {
		t.IPv4PerServer = 1
	}
	if t.IPv6PerServer == 0 {
		t.IPv6PerServer = 1
	}
	if t.EgressKind == "" {
		t.EgressKind = "cold_smtp"
	}
	return t
}

func (h *Handler) AdminListProvisioningTemplates(c *gin.Context) {
	if h.ProvisioningTemplateRepo == nil {
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
		return
	}
	rows, err := h.ProvisioningTemplateRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]provTemplateDTO, 0, len(rows))
	for i := range rows {
		out = append(out, toTemplateDTO(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) AdminGetProvisioningTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	row, err := h.ProvisioningTemplateRepo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, toTemplateDTO(row))
}

func (h *Handler) AdminCreateProvisioningTemplate(c *gin.Context) {
	if h.ProvisioningTemplateRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "template repo not configured"})
		return
	}
	var d provTemplateDTO
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	t := fromTemplateDTO(&d)
	if t.Name == "" || t.Provider == "" || t.Location == "" || t.ServerType == "" || t.Tier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, provider, location, server_type and worker_tier are required"})
		return
	}
	if !repository.IsClientRequestableTier(t.Tier) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "dedicated tier templates cannot be created; dedicated workers are allocated automatically by the control plane",
			"code":  "tier_not_allowed",
		})
		return
	}
	if err := h.ProvisioningTemplateRepo.Create(c.Request.Context(), t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toTemplateDTO(t))
}

func (h *Handler) AdminUpdateProvisioningTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var d provTemplateDTO
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	t := fromTemplateDTO(&d)
	t.ID = id
	if t.Name == "" || t.Provider == "" || t.Location == "" || t.ServerType == "" || t.Tier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, provider, location, server_type and worker_tier are required"})
		return
	}
	if !repository.IsClientRequestableTier(t.Tier) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "templates cannot be set to the dedicated tier; dedicated workers are allocated automatically by the control plane",
			"code":  "tier_not_allowed",
		})
		return
	}
	if err := h.ProvisioningTemplateRepo.Update(c.Request.Context(), t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toTemplateDTO(t))
}

func (h *Handler) AdminDeleteProvisioningTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.ProvisioningTemplateRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Provisioning jobs
// ---------------------------------------------------------------------------

type CreateProvisioningJobRequest struct {
	TemplateID  *uuid.UUID      `json:"template_id,omitempty"`
	Custom      json.RawMessage `json:"custom,omitempty"`
	TriggeredBy string          `json:"triggered_by,omitempty"`
}

// Job rows are flat in the repo and the UI works in a nested {config:{...}}
// shape with snake_case keys — the same translation problem the templates have.
// toJobDTO maps the row (and decodes the stored flat config snapshot back into
// the nested ProvisioningConfig) so the jobs list / detail / progress panel get
// the fields they read.
type provJobStepDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type provJobTimelineDTO struct {
	State string    `json:"state"`
	At    time.Time `json:"at"`
	Note  string    `json:"note,omitempty"`
}

type provJobDTO struct {
	ID               uuid.UUID             `json:"id"`
	State            string                `json:"state"`
	TriggeredBy      string                `json:"triggered_by,omitempty"`
	Provider         string                `json:"provider"`
	TemplateID       *uuid.UUID            `json:"template_id,omitempty"`
	TemplateName     string                `json:"template_name,omitempty"`
	Config           provTemplateConfigDTO `json:"config"`
	Progress         []provJobStepDTO      `json:"progress"`
	Timeline         []provJobTimelineDTO  `json:"timeline"`
	CreatedWorkerIDs []uuid.UUID           `json:"created_worker_ids,omitempty"`
	LastError        *string               `json:"last_error,omitempty"`
	CompletedAt      *time.Time            `json:"completed_at,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

func toJobDTO(j *repository.ProvisioningJob) provJobDTO {
	cfg := provTemplateConfigDTO{Labels: []provLabelDTO{}}
	templateName := ""
	if len(j.Config) > 0 {
		var flat repository.ProvisioningTemplate
		if err := json.Unmarshal(j.Config, &flat); err == nil {
			cfg = toTemplateDTO(&flat).Config
			// The snapshot carries the template name for template-launched jobs
			// (custom jobs snapshot under the placeholder name "custom").
			if j.TemplateID != nil {
				templateName = flat.Name
			}
		}
	}
	return provJobDTO{
		ID:               j.ID,
		State:            string(j.State),
		TriggeredBy:      j.TriggeredBy,
		Provider:         j.Provider,
		TemplateID:       j.TemplateID,
		TemplateName:     templateName,
		Config:           cfg,
		Progress:         []provJobStepDTO{},
		Timeline:         []provJobTimelineDTO{},
		CreatedWorkerIDs: j.WorkerIDs,
		LastError:        j.Error,
		CompletedAt:      j.CompletedAt,
		CreatedAt:        j.CreatedAt,
		UpdatedAt:        j.UpdatedAt,
	}
}

func (h *Handler) AdminListProvisioningJobs(c *gin.Context) {
	if h.ProvisioningJobRepo == nil {
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := h.ProvisioningJobRepo.List(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]provJobDTO, 0, len(rows))
	for i := range rows {
		out = append(out, toJobDTO(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) AdminGetProvisioningJob(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	row, err := h.ProvisioningJobRepo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, toJobDTO(row))
}

func (h *Handler) AdminCreateProvisioningJob(c *gin.Context) {
	if h.ProvisioningJobRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provisioning jobs repo not configured"})
		return
	}
	var req CreateProvisioningJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	// Resolve template or custom config into the JSONB config column.
	var (
		config     json.RawMessage
		templateID *uuid.UUID
		provider   string
	)
	if req.TemplateID != nil {
		if h.ProvisioningTemplateRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "template repo not configured"})
			return
		}
		t, err := h.ProvisioningTemplateRepo.Get(c.Request.Context(), *req.TemplateID)
		if err != nil || t == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
			return
		}
		if !repository.IsClientRequestableTier(t.Tier) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "this template targets the dedicated tier, which is allocated automatically by the control plane; pick a shared-tier template",
				"code":  "tier_not_allowed",
			})
			return
		}
		// Snapshot the flat template into the job's config column. Its field
		// names line up with provisioning.JobConfig, so the state machine reads
		// it directly.
		b, _ := json.Marshal(t)
		config = b
		templateID = &t.ID
		provider = t.Provider
	} else {
		if len(req.Custom) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "either template_id or custom config required"})
			return
		}
		// The UI sends the nested config shape (worker_tier, {key,value} label
		// rows). Normalize it through the same mapping templates use so the
		// snapshot matches provisioning.JobConfig.
		var cfgDTO provTemplateConfigDTO
		if err := json.Unmarshal(req.Custom, &cfgDTO); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid custom config: " + err.Error()})
			return
		}
		if cfgDTO.Provider == "" || cfgDTO.Location == "" || cfgDTO.ServerType == "" || cfgDTO.WorkerTier == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "custom config requires provider, location, server_type and worker_tier"})
			return
		}
		snap := fromTemplateDTO(&provTemplateDTO{Name: "custom", Config: cfgDTO})
		if !repository.IsClientRequestableTier(snap.Tier) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "dedicated tier cannot be provisioned directly; dedicated workers are allocated automatically by the control plane",
				"code":  "tier_not_allowed",
			})
			return
		}
		b, _ := json.Marshal(snap)
		config = b
		provider = cfgDTO.Provider
	}

	if provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider missing from config"})
		return
	}

	// Cloud-first gate: a provisioning job for a provider can only be created
	// once a cloud credential for that provider exists — otherwise the job
	// would sit in 'pending' forever with no way to reach the provider API.
	var credentialID *uuid.UUID
	if h.CloudCredentialRepo != nil {
		cred, err := h.CloudCredentialRepo.GetByProvider(c.Request.Context(), provider)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if cred == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "no cloud provider connected for " + provider + " — add one under Settings → Cloud Providers first",
				"code":  "cloud_provider_required",
			})
			return
		}
		credentialID = &cred.ID
	}

	triggeredBy := req.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = "admin"
	}

	job := &repository.ProvisioningJob{
		State:        models.ProvJobPending,
		TriggeredBy:  triggeredBy,
		Provider:     provider,
		CredentialID: credentialID,
		TemplateID:   templateID,
		Config:       config,
	}
	if err := h.ProvisioningJobRepo.Create(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// The state machine pickup is asynchronous: the provisioning runner
	// (internal/app/provisioning Runner) processes jobs in state !=
	// completed/failed. The admin UI polls GET /admin/provisioning-jobs/:id
	// for live status.

	c.JSON(http.StatusAccepted, toJobDTO(job))
}

// AdminRetryProvisioningJob resets a failed job back to pending so the runner
// re-attempts it.
func (h *Handler) AdminRetryProvisioningJob(c *gin.Context) {
	if h.ProvisioningJobRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provisioning jobs repo not configured"})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	job, err := h.ProvisioningJobRepo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if job.State != models.ProvJobFailed {
		c.JSON(http.StatusConflict, gin.H{
			"error": "only failed jobs can be retried",
			"code":  "job_not_retryable",
		})
		return
	}
	if err := h.ProvisioningJobRepo.Retry(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	job, err = h.ProvisioningJobRepo.Get(c.Request.Context(), id)
	if err != nil || job == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, toJobDTO(job))
}

// ---------------------------------------------------------------------------
// Provisioning policy
// ---------------------------------------------------------------------------

func (h *Handler) AdminListProvisioningPolicy(c *gin.Context) {
	if h.ProvisioningPolicyRepo == nil {
		c.JSON(http.StatusOK, gin.H{"policies": []any{}})
		return
	}
	rows, err := h.ProvisioningPolicyRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": rows})
}

func (h *Handler) AdminUpdateProvisioningPolicy(c *gin.Context) {
	if h.ProvisioningPolicyRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "policy repo not configured"})
		return
	}
	var p repository.ProvisioningPolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if p.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider required"})
		return
	}
	if err := h.ProvisioningPolicyRepo.Update(c.Request.Context(), &p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}
