package postmaster

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bitan-del/dakiya-mail/internal/config"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

const (
	// SyncDays is the daily refresh window: wide enough to pick up Gmail's late corrections.
	SyncDays = 30
	// BackfillDays is the first pull after connecting; Gmail keeps about 120 days.
	BackfillDays = 120
	// DefaultDays is the report window when the caller names none.
	DefaultDays = 30

	connectSyncBudget = 20 * time.Second
)

var (
	ErrInstanceScoped = errx.NewWithIdentifier(errx.Forbidden, "postmaster_instance_scoped", "Google Postmaster credentials are shared by the whole instance, so only a self-hosted instance can connect them from the dashboard.")
	ErrCredentials    = errx.NewWithIdentifier(errx.Unprocessable, "postmaster_credentials_invalid", "Google rejected these credentials. Check the client ID, client secret and refresh token.")
	ErrUnreachable    = errx.NewWithIdentifier(errx.ServiceUnavailable, "postmaster_unreachable", "Google could not be reached to check these credentials. Try again shortly.")
	ErrNoEncryption   = errx.NewWithIdentifier(errx.NotImplemented, "postmaster_encryption_unconfigured", "Set CREDENTIALS_ENCRYPTION_KEY before connecting Google Postmaster Tools, so the credentials can be stored encrypted.")
	ErrDomainNotFound = errx.NewWithIdentifier(errx.NotFound, "postmaster_domain_not_found", "That domain is not a sending domain of this workspace.")
	ErrInvalidDays    = errx.NewWithIdentifier(errx.BadRequest, "postmaster_invalid_days", "days must be a whole number from 1 to 120.")
)

// Service pulls Gmail Postmaster stats and reports them per workspace.
type Service interface {
	// Report answers GET /deliverability/postmaster. domain "" picks the first domain with data.
	Report(ctx context.Context, orgID uuid.UUID, domain string, days int) (*models.PostmasterReport, *errx.Error)
	Connect(ctx context.Context, creds models.PostmasterCredentials) *errx.Error
	Disconnect(ctx context.Context) *errx.Error
	// Sync refreshes every workspace's domains. Google-side failures are recorded
	// on the connection, not returned: only storage failures are errors.
	Sync(ctx context.Context, days int) error
}

type service struct {
	repo      repository.PostmasterRepository
	newClient func(context.Context, models.PostmasterCredentials) *Client
}

func NewService(repo repository.PostmasterRepository) Service {
	return &service{repo: repo, newClient: NewClient}
}

func (s *service) Report(ctx context.Context, orgID uuid.UUID, domain string, days int) (*models.PostmasterReport, *errx.Error) {
	if days < 1 || days > BackfillDays {
		return nil, ErrInvalidDays
	}
	conn, err := s.repo.GetConnection(ctx)
	if err != nil {
		log.Error().Err(err).Msg("postmaster: read connection")
		return nil, errx.InternalError()
	}
	domains, err := s.repo.OrgDomains(ctx, orgID)
	if err != nil {
		return nil, errx.InternalError()
	}

	picked, ok := pickDomain(domains, domain)
	if !ok {
		return nil, ErrDomainNotFound
	}

	rep := &models.PostmasterReport{
		CanConnect: canManage(),
		Domains:    domains,
		Domain:     picked,
		Days:       days,
		Thresholds: models.PostmasterThresholds{SpamWarn: models.PostmasterSpamWarn, SpamCritical: models.PostmasterSpamCritical},
		Data:       []models.PostmasterDay{},
	}
	if conn != nil {
		rep.Connected, rep.LastSyncedAt, rep.LastError = true, conn.SyncedAt, conn.LastError
	}
	if picked != "" {
		if rep.Data, err = s.repo.ListStats(ctx, orgID, picked, days); err != nil {
			return nil, errx.InternalError()
		}
	}
	rep.Summary = Summarize(rep.Data)
	return rep, nil
}

// pickDomain resolves the requested domain, defaulting to the first one that
// has data and then to the first one. "" with no domains is fine: nothing to pick.
func pickDomain(domains []models.PostmasterDomain, want string) (string, bool) {
	want = strings.ToLower(strings.TrimSpace(want))
	if want != "" {
		for _, d := range domains {
			if d.Domain == want {
				return want, true
			}
		}
		return "", false
	}
	for _, d := range domains {
		if d.LatestDate != nil {
			return d.Domain, true
		}
	}
	if len(domains) > 0 {
		return domains[0].Domain, true
	}
	return "", true
}

// Summarize aggregates the days that carried at least one metric. Gmail gives
// no volumes, so averages are plain means over reporting days.
func Summarize(days []models.PostmasterDay) models.PostmasterSummary {
	sorted := append([]models.PostmasterDay(nil), days...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })

	var sum models.PostmasterSummary
	var spam, spf, dkim, dmarc mean
	for _, d := range sorted {
		if d.DomainReputation == nil && d.SpamRate == nil && d.SPFRatio == nil && d.DKIMRatio == nil && d.DMARCRatio == nil {
			continue
		}
		sum.DaysWithData++
		date := d.Date
		sum.LatestDate = &date
		if d.DomainReputation != nil {
			rep := *d.DomainReputation
			sum.LatestReputation = &rep
		}
		spf.add(d.SPFRatio)
		dkim.add(d.DKIMRatio)
		dmarc.add(d.DMARCRatio)
		if d.SpamRate != nil {
			spam.add(d.SpamRate)
			if *d.SpamRate >= models.PostmasterSpamWarn {
				sum.DaysOverWarn++
			}
			if *d.SpamRate >= models.PostmasterSpamCritical {
				sum.DaysOverCritical++
			}
		}
	}
	sum.SpamRateAvg, sum.SpamRateMax = spam.avg(), spam.max
	sum.SPFAvg, sum.DKIMAvg, sum.DMARCAvg = spf.avg(), dkim.avg(), dmarc.avg()
	return sum
}

// mean accumulates the non-nil values of one metric.
type mean struct {
	total float64
	n     int
	max   *float64
}

func (m *mean) add(v *float64) {
	if v == nil {
		return
	}
	m.total += *v
	m.n++
	if m.max == nil || *v > *m.max {
		x := *v
		m.max = &x
	}
}

func (m *mean) avg() *float64 {
	if m.n == 0 {
		return nil
	}
	v := m.total / float64(m.n)
	return &v
}

// canManage: the credentials are one per instance, so on a shared hosted
// instance no single workspace may replace them.
func canManage() bool { return config.SelfHosted() }

func (s *service) Connect(ctx context.Context, creds models.PostmasterCredentials) *errx.Error {
	if !canManage() {
		return ErrInstanceScoped
	}
	creds.ClientID = strings.TrimSpace(creds.ClientID)
	creds.ClientSecret = strings.TrimSpace(creds.ClientSecret)
	creds.RefreshToken = strings.TrimSpace(creds.RefreshToken)
	if creds.ClientID == "" || creds.ClientSecret == "" || creds.RefreshToken == "" {
		return ErrCredentials
	}

	if err := s.newClient(ctx, creds).Check(); err != nil {
		if errors.Is(err, ErrAccount) {
			return ErrCredentials
		}
		return ErrUnreachable
	}
	if err := s.repo.PutCredentials(ctx, creds); err != nil {
		if errors.Is(err, repository.ErrPostmasterNoEncrypter) {
			return ErrNoEncryption
		}
		return errx.InternalError()
	}

	// A first pull so the page has data now; whatever the budget cuts off, the daily job finishes.
	syncCtx, cancel := context.WithTimeout(ctx, connectSyncBudget)
	defer cancel()
	if err := s.Sync(syncCtx, BackfillDays); err != nil {
		log.Warn().Err(err).Msg("postmaster: first sync after connect")
	}
	return nil
}

func (s *service) Disconnect(ctx context.Context) *errx.Error {
	if !canManage() {
		return ErrInstanceScoped
	}
	if err := s.repo.DeleteCredentials(ctx); err != nil {
		return errx.InternalError()
	}
	return nil
}

func (s *service) Sync(ctx context.Context, days int) error {
	conn, err := s.repo.GetConnection(ctx)
	if err != nil {
		return err
	}
	if conn == nil {
		return nil
	}
	targets, err := s.repo.MailboxDomains(ctx)
	if err != nil {
		return err
	}

	// One Google call per distinct domain, however many workspaces send from it.
	orgsByDomain := map[string][]uuid.UUID{}
	for _, t := range targets {
		orgsByDomain[t.Domain] = append(orgsByDomain[t.Domain], t.OrganizationID)
	}
	domains := make([]string, 0, len(orgsByDomain))
	for d := range orgsByDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	to := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour) // Gmail has nothing for today
	from := to.AddDate(0, 0, -(days - 1))
	client := s.newClient(ctx, conn.Credentials)

	var failure string
	for _, domain := range domains {
		stats, err := client.Stats(ctx, domain, from, to)
		if errors.Is(err, ErrNoAccess) {
			continue // not registered or not verified in Postmaster Tools: nothing to pull
		}
		if errors.Is(err, ErrAccount) {
			failure = err.Error()
			break
		}
		if err != nil {
			failure = err.Error()
			continue
		}
		for _, org := range orgsByDomain[domain] {
			if err := s.repo.UpsertStats(ctx, org, domain, stats); err != nil {
				return err
			}
		}
	}
	return s.repo.SetSyncResult(ctx, time.Now(), failure)
}
