package handler

import (
	"github.com/bitan-del/dakiya-mail/internal/app/admin"
	"github.com/bitan-del/dakiya-mail/internal/app/adminoutreach"
	"github.com/bitan-del/dakiya-mail/internal/app/advanced"
	"github.com/bitan-del/dakiya-mail/internal/app/advisor"
	"github.com/bitan-del/dakiya-mail/internal/app/aiagent"
	"github.com/bitan-del/dakiya-mail/internal/app/aitools"
	"github.com/bitan-del/dakiya-mail/internal/app/analytics"
	"github.com/bitan-del/dakiya-mail/internal/app/apikey"
	"github.com/bitan-del/dakiya-mail/internal/app/audit"
	"github.com/bitan-del/dakiya-mail/internal/app/auth"
	"github.com/bitan-del/dakiya-mail/internal/app/behavior"
	"github.com/bitan-del/dakiya-mail/internal/app/bootstrap"
	"github.com/bitan-del/dakiya-mail/internal/app/campaign"
	"github.com/bitan-del/dakiya-mail/internal/app/cloudlink"
	"github.com/bitan-del/dakiya-mail/internal/app/compose"
	"github.com/bitan-del/dakiya-mail/internal/app/contact"
	"github.com/bitan-del/dakiya-mail/internal/app/credits"
	"github.com/bitan-del/dakiya-mail/internal/app/crm"
	"github.com/bitan-del/dakiya-mail/internal/app/dangerzone"
	"github.com/bitan-del/dakiya-mail/internal/app/discount"
	"github.com/bitan-del/dakiya-mail/internal/app/email"
	"github.com/bitan-del/dakiya-mail/internal/app/emailsend"
	emailverifyapp "github.com/bitan-del/dakiya-mail/internal/app/emailverify"
	"github.com/bitan-del/dakiya-mail/internal/app/feature"
	"github.com/bitan-del/dakiya-mail/internal/app/group"
	"github.com/bitan-del/dakiya-mail/internal/app/instancecheck"
	"github.com/bitan-del/dakiya-mail/internal/app/instanceconfig"
	"github.com/bitan-del/dakiya-mail/internal/app/instancesettings"
	"github.com/bitan-del/dakiya-mail/internal/app/integration"
	"github.com/bitan-del/dakiya-mail/internal/app/leadsync"
	"github.com/bitan-del/dakiya-mail/internal/app/mcp"
	"github.com/bitan-del/dakiya-mail/internal/app/notification"
	"github.com/bitan-del/dakiya-mail/internal/app/oauth"
	"github.com/bitan-del/dakiya-mail/internal/app/organization"
	"github.com/bitan-del/dakiya-mail/internal/app/orgrisk"
	"github.com/bitan-del/dakiya-mail/internal/app/orgtransfer"
	"github.com/bitan-del/dakiya-mail/internal/app/passkey"
	"github.com/bitan-del/dakiya-mail/internal/app/placement"
	"github.com/bitan-del/dakiya-mail/internal/app/poollink"
	"github.com/bitan-del/dakiya-mail/internal/app/postmaster"
	"github.com/bitan-del/dakiya-mail/internal/app/preflight"
	"github.com/bitan-del/dakiya-mail/internal/app/ratelimit"
	"github.com/bitan-del/dakiya-mail/internal/app/referral"
	"github.com/bitan-del/dakiya-mail/internal/app/releases"
	"github.com/bitan-del/dakiya-mail/internal/app/research"
	"github.com/bitan-del/dakiya-mail/internal/app/salesblink"
	"github.com/bitan-del/dakiya-mail/internal/app/sequence"
	"github.com/bitan-del/dakiya-mail/internal/app/skills"
	"github.com/bitan-del/dakiya-mail/internal/app/socket"
	"github.com/bitan-del/dakiya-mail/internal/app/stripe"
	"github.com/bitan-del/dakiya-mail/internal/app/subscription"
	"github.com/bitan-del/dakiya-mail/internal/app/sysstatus"
	"github.com/bitan-del/dakiya-mail/internal/app/team"
	"github.com/bitan-del/dakiya-mail/internal/app/template"
	"github.com/bitan-del/dakiya-mail/internal/app/token"
	"github.com/bitan-del/dakiya-mail/internal/app/trial"
	"github.com/bitan-del/dakiya-mail/internal/app/twofa"
	"github.com/bitan-del/dakiya-mail/internal/app/tz"
	"github.com/bitan-del/dakiya-mail/internal/app/unibox"
	"github.com/bitan-del/dakiya-mail/internal/app/user"
	"github.com/bitan-del/dakiya-mail/internal/app/warmup"
	"github.com/bitan-del/dakiya-mail/internal/app/warmupcontent"
	"github.com/bitan-del/dakiya-mail/internal/app/webhook"
	"github.com/bitan-del/dakiya-mail/internal/app/websitetracking"
	"github.com/bitan-del/dakiya-mail/internal/app/worker"
	"github.com/bitan-del/dakiya-mail/internal/app/worker_orchestrator"
	"github.com/bitan-del/dakiya-mail/internal/pkg/generation"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/encryptedkeys"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/pubsub"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/storage"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/notify"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/bitan-del/dakiya-mail/internal/tasks"
)

type Handler struct {
	AuthService    auth.AuthService
	TokenService   token.TokenService
	PasskeyService passkey.Service

	// Native-app social sign-in discovery (GET /auth/providers).
	ExternalAuthProviders models.ExternalAuthProviders

	// Deployment facts served by GET /auth/config so the login screen renders
	// what this backend can actually do instead of guessing. The sign-in
	// provider list is not here: it comes from the auth service, which is what
	// actually holds the configured flows.
	MailDelivers   bool
	PasskeysUsable bool
	MailTransport  string
	// MailTransportRef backs the admin mail diagnostics, which need Preflight
	// and so cannot go through the EmailNotificationService interface.
	MailTransportRef *notify.Transport
	// BootstrapService backs the first-run setup page.
	BootstrapService *bootstrap.Service
	UserService      user.UserService
	EmailService     email.EmailService
	CampaignService  campaign.CampaignService
	ContactService   contact.ContactService
	SequenceService  sequence.SequenceService
	UniboxService    unibox.UniboxService

	FolderService   group.GroupService
	TagService      group.GroupService
	CategoryService group.GroupService

	TzService           tz.TzService
	SocketService       socket.SocketService
	TasksService        tasks.TasksService
	NotificationService notification.Service
	TwoFAService        twofa.Service

	// New services
	APIKeyService    apikey.APIKeyService
	AnalyticsService analytics.AnalyticsService
	AuditService     audit.AuditService
	RateLimitService ratelimit.RateLimitService

	// Subscription & billing
	SubscriptionService subscription.SubscriptionService
	StripeService       stripe.StripeService
	DiscountService     discount.DiscountService
	ReferralService     referral.Service

	// Trial & feature gates
	TrialService            trial.TrialService
	FeatureGateService      feature.FeatureGateService
	WorkerAssignmentService worker.WorkerAssignmentService

	// Organization & IAM
	OrganizationService organization.OrganizationService
	OrgRiskService      orgrisk.Service

	// CRM
	CRMService crm.CRMService

	// Teams
	TeamService team.TeamService

	// Email send & templates
	TemplateService  template.TemplateService
	EmailSendService emailsend.EmailSendService

	// Compose mailbox picker: scores the org's mailboxes for a recipient
	// (affinity, budget, auth health) and backs auto selection.
	ComposeService compose.Service

	// Admin
	AdminService         admin.AdminService
	AdminOutreachService adminoutreach.Service

	// Worker orchestration (SSH-driven lifecycle for admin-managed workers)
	WorkerOrchestrator *worker_orchestrator.Orchestrator
	WorkerRepo         repository.WorkerRepository
	CredentialsRepo    repository.CredentialsRepository
	ReleasesService    *releases.Service

	// Notifications
	EmailNotificationService notify.EmailNotificationService

	// Advanced outreach controls
	AdvancedService advanced.Service

	// Website tracking snippet: settings and the page-view ingest path.
	WebsiteTrackingService websitetracking.Service

	// Pre-send email verification (control-plane SMTP RCPT probe / pluggable
	// paid backend). Drops hard-bouncing addresses before a worker sends.
	EmailVerifyService emailverifyapp.Service

	// Per-mailbox human sending behaviour: the ranges a mailbox rolls its
	// workday from, and the plan it rolled for today. Nil disables the
	// behaviour endpoints (the schedulers then run every mailbox on its fixed
	// cap and gap).
	BehaviorService behavior.Service

	// Warmup health
	WarmupService warmup.Service

	// Warmup routing rules — customer-defined preferences for premium-pool
	// partner selection (e.g. Gmail recipients from Google Workspace senders).
	WarmupRoutingRepo repository.WarmupRoutingRepository

	// Warmup content bank + offline AI generator (admin control/visibility).
	WarmupContentRepo    repository.WarmupContentRepository
	WarmupContentService warmupcontent.Service

	// AI writing assistant + credit ledger.
	CreditService    credits.CreditService
	WritingGenerator generation.WritingGenerator

	// AI provider layer (RunAgent tool-loop backend) + pluggable web search,
	// shared by the dashboard agent, research, automation AI nodes, and the
	// inbox agent. Nil when no LLM provider is configured.
	AIProvider generation.Provider
	AISearch   generation.SearchClient
	// AIEnv is the environment's AI selection at boot, the fallback the admin AI test layers saved settings over.
	AIEnv generation.ProviderSettings

	// AITools is the shared tool registry the dashboard agent and MCP server
	// run on. Handlers bound to the invoking user's permissions.
	AITools *aitools.Registry

	// AIAgentService orchestrates the dashboard agent (sessions, SSE runs,
	// approvals, per-iteration credits). Nil when no LLM provider is configured.
	AIAgentService aiagent.Service

	// ResearchService runs the AI contact-research agent (sync + batch).
	ResearchService research.Service

	// SkillsService manages org AI skills (playbooks) and injects them into AI
	// prompts.
	SkillsService skills.Service

	// MCPService manages org-connected MCP servers (external tools).
	MCPService mcp.Service

	// AIDraftRepo stores inbox-agent reply drafts awaiting human review (M10).
	// The draft is created in the consumer; these list/approve/discard handlers
	// read + resolve it. Nil disables the review endpoints.
	AIDraftRepo repository.AIDraftRepository

	// Seed inbox-placement testing.
	PlacementRepo    repository.PlacementRepository
	PlacementService placement.Service

	// Advisor: continuously-evaluated recommendations about deliverability,
	// mailbox config, warmup, campaign performance, copy, and list hygiene.
	// The repository is held alongside the service so a read can cheaply check
	// how stale the findings are before deciding to re-evaluate inline.
	AdvisorService    advisor.Service
	AdvisorRepository repository.AdvisorRepository

	// Pre-send deliverability gate for a campaign (GET /campaigns/:id/preflight).
	PreflightService preflight.Service

	// Customer-facing webhooks (subscribe → HMAC-signed delivery).
	WebhookService webhook.Service

	// Third-party integrations (Calendly, Cal.com, DMARC, Postmaster,
	// SNDS, Cloudflare, GoDaddy, Namecheap, Google Sheets).
	IntegrationService integration.Service
	ContactRepo        repository.ContactRepository

	// OAuth 2.1 authorization server (third-party app registration + the
	// authorization-code-with-PKCE flow + bearer-token validation).
	OAuthService *oauth.Service

	// Realtime publisher for handler paths that emit live dashboard events
	// directly (inbound meeting webhooks have no service layer of their own).
	// nil-safe: realtime is a nicety, not a requirement.
	StreamingPublisher *pubsub.StreamingPublisher

	// On-demand Google Sheets -> leads sync. Reuses the google_sheets OAuth
	// connection's token to read sheets and the contact import path to upsert.
	LeadSyncService leadsync.Service

	// Public websocket URL used by frontend clients
	WebsocketURI string

	// Object storage for user-uploaded artifacts (avatars, etc.).
	Storage storage.Store

	// Encrypted-DEK store. Served to workers over HTTPS at
	// /api/v1/internal/dek/:userID so workers don't need direct Postgres
	// access. Backend processes use Postgres directly; workers use the
	// HTTP-proxy implementation.
	EncryptedKeys encryptedkeys.Store

	// Worker messageId -> internal email map, served to workers over HTTPS at
	// /api/v1/internal/email-message-map for the same no-direct-Postgres reason
	// as EncryptedKeys. Backed by Postgres in the backend.
	EmailMessageMap repository.EmailMessageMapRepository

	// Mailbox sync state, read by the dashboard (GET /emails/:id/sync) and by
	// the worker's priority lane over /api/v1/internal/sync/own-conversation.
	EmailSyncState repository.EmailSyncStateRepository

	// Click-link store, served to the tracking service over HTTPS at
	// /api/v1/internal/tracked-links/:id (same no-direct-Postgres rule).
	TrackedLinks repository.TrackedLinkRepository

	// Direct repositories used by handlers that don't yet have a
	// service layer (avatars, etc.). Keep narrow and add a service
	// only when business logic accumulates.
	UserRepo                 repository.UserRepository
	OrgRepo                  repository.OrganizationRepository
	AttachmentRepo           repository.AttachmentRepository
	StorageBackendRepo       repository.StorageBackendRepository
	CloudCredentialRepo      repository.CloudCredentialRepository
	ProvisioningTemplateRepo repository.ProvisioningTemplateRepository
	ProvisioningJobRepo      repository.ProvisioningJobRepository
	ProvisioningPolicyRepo   repository.ProvisioningPolicyRepository

	// Danger zone (delayed deletions for orgs & user accounts)
	DangerZoneService dangerzone.Service

	// Workspace archives: export an organization to a portable file and import
	// one back, for moving between instances. Nil disables the endpoints.
	OrgTransferService orgtransfer.Service

	// PoolLinkService (cloud side) and CloudLinkService (self-hosted side) are nil-safe: routes answer 501.
	PoolLinkService  poollink.Service
	CloudLinkService cloudlink.Service

	// PostmasterService reads Gmail Postmaster Tools stats. Nil-safe: routes answer 501.
	PostmasterService postmaster.Service

	// SalesBlinkService hands warmup to a SalesBlink account instead of the
	// local pool. Nil-safe: routes answer 501.
	SalesBlinkService salesblink.Service

	// Infrastructure liveness probes for the admin System Status page.
	// Wired in cmd/backend/main.go where the concrete clients live.
	SystemChecker *sysstatus.Checker

	// Operator visibility (admin panel, Instance section).
	//
	// InstanceRuntime carries the facts only boot knows (the resolved CORS
	// list, the WebAuthn RP, the OIDC redirect, the auth policy) so the
	// configuration page shows what the process actually derived. Nil is safe:
	// every resolver falls back to the environment.
	InstanceRuntime *instanceconfig.Runtime
	// InstanceChecks is the setup and health registry. Nil falls back to the
	// environment-only checks, so the page is never blank.
	InstanceChecks *instancecheck.Registry
	// InstanceSettings is the database-backed settings tier. It holds only
	// keys no environment variable owns.
	InstanceSettings instancesettings.Service
}
