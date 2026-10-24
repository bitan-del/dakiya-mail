// Package salesblink hands mailbox warmup to a SalesBlink account.
//
// Why: a self-hosted instance is a single organization, so its local warmup
// pool can only pair its own mailboxes with each other, which providers learn
// to recognize. SalesBlink runs a large shared pool, so enrolling there gives
// warmup mail real, varied counterparties.
//
// The cost: enrolling sends the mailbox's SMTP/IMAP credentials to SalesBlink.
// That is unavoidable — SalesBlink has to log in as the mailbox to send warmup
// mail, read replies, and pull messages out of spam. Mailboxes connected with
// OAuth cannot be enrolled at all (see ErrOAuthMailbox).
package salesblink

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

var (
	ErrNotConnected = errx.NewWithIdentifier(errx.Conflict, "salesblink_not_connected",
		"This instance is not connected to SalesBlink. Add an API key in Settings > SalesBlink.")
	ErrOAuthMailbox = errx.NewWithIdentifier(errx.Unprocessable, "salesblink_oauth_mailbox",
		"Google and Microsoft sign-in mailboxes cannot be warmed by SalesBlink: it needs SMTP and IMAP credentials, and an OAuth mailbox's grant is bound to this instance. Reconnect the mailbox with SMTP/IMAP (an app password) to enroll it.")
	ErrNoIMAP = errx.NewWithIdentifier(errx.Unprocessable, "salesblink_no_imap",
		"This mailbox has no IMAP settings. SalesBlink warmup needs IMAP to read replies and rescue mail from spam.")
	ErrMailboxInactive = errx.NewWithIdentifier(errx.Unprocessable, "salesblink_mailbox_inactive",
		"Only active mailboxes can be enrolled.")
)

// Service is the SalesBlink warmup integration.
type Service interface {
	Status(ctx context.Context, orgID uuid.UUID) (*models.SalesBlinkStatus, *errx.Error)
	// Connect verifies the key against SalesBlink before storing it.
	Connect(ctx context.Context, userID uuid.UUID, apiKey, baseURL string, autoEnroll bool) (*models.SalesBlinkLink, *errx.Error)
	UpdateSettings(ctx context.Context, autoEnroll bool, perDay, maxDaily, rampUp int, autoRampUp bool) (*models.SalesBlinkLink, *errx.Error)
	Disconnect(ctx context.Context) *errx.Error

	ListMailboxes(ctx context.Context, orgID uuid.UUID) ([]models.SalesBlinkMailboxRow, *errx.Error)
	Enroll(ctx context.Context, orgID, accountID uuid.UUID) (*models.SalesBlinkMailboxRow, *errx.Error)
	Unenroll(ctx context.Context, orgID, accountID uuid.UUID) *errx.Error

	// SyncAll enrolls every eligible, not-yet-enrolled mailbox. This is the
	// "turn it on for all 200" path: called from the UI, and on a timer.
	SyncAll(ctx context.Context, orgID uuid.UUID) (*models.SalesBlinkSyncResult, *errx.Error)
	// AutoEnroll is the hook for a freshly connected mailbox. It never returns
	// an error: a warmup enrollment must not fail the mailbox connection.
	AutoEnroll(ctx context.Context, accountID uuid.UUID)
	// RefreshHealth pulls health scores for enrolled mailboxes.
	RefreshHealth(ctx context.Context) error

	// IsEnrolled tells the local warmup scheduler to stand down for this
	// mailbox. Fails closed to false so a lookup error never silently stops
	// local warmup.
	IsEnrolled(ctx context.Context, accountID uuid.UUID) bool
}

type service struct {
	repo   repository.SalesBlinkRepository
	emails repository.EmailRepository

	// Guards SyncAll so a timer tick and a UI click cannot double-enroll.
	syncMu sync.Mutex
}

func NewService(repo repository.SalesBlinkRepository, emails repository.EmailRepository) Service {
	return &service{repo: repo, emails: emails}
}

func (s *service) link(ctx context.Context) (*models.SalesBlinkLink, *errx.Error) {
	l, err := s.repo.Get(ctx)
	if err != nil {
		return nil, errx.InternalError()
	}
	if l == nil {
		return nil, ErrNotConnected
	}
	return l, nil
}

func (s *service) clientFor(l *models.SalesBlinkLink) *client {
	base := l.BaseURL
	if strings.TrimSpace(base) == "" {
		base = models.SalesBlinkDefaultBaseURL
	}
	return newClient(base, l.APIKey)
}

// ─── connection ──────────────────────────────────────────────────────────

func (s *service) Connect(ctx context.Context, userID uuid.UUID, apiKey, baseURL string, autoEnroll bool) (*models.SalesBlinkLink, *errx.Error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errx.NewWithIdentifier(errx.BadRequest, "salesblink_key_required", "An API key is required.")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = models.SalesBlinkDefaultBaseURL
	}

	// Verify before storing, so a bad key fails here rather than silently on
	// the first enrollment.
	probe := newClient(baseURL, apiKey)
	if xerr := probe.do(ctx, http.MethodGet, "/account/verify", nil, nil); xerr != nil {
		return nil, xerr
	}

	l := &models.SalesBlinkLink{
		BaseURL:           baseURL,
		APIKey:            apiKey,
		AutoEnroll:        autoEnroll,
		WarmupPerDay:      5,
		MaxDailyFrequency: 30,
		RampUpFrequency:   1,
		AutoRampUp:        true,
		ConnectedBy:       &userID,
	}
	// Preserve previously tuned volumes across a key rotation.
	if prev, err := s.repo.Get(ctx); err == nil && prev != nil {
		l.WarmupPerDay, l.MaxDailyFrequency = prev.WarmupPerDay, prev.MaxDailyFrequency
		l.RampUpFrequency, l.AutoRampUp = prev.RampUpFrequency, prev.AutoRampUp
	}
	if err := s.repo.Put(ctx, l); err != nil {
		return nil, errx.InternalError()
	}
	return l, nil
}

func (s *service) UpdateSettings(ctx context.Context, autoEnroll bool, perDay, maxDaily, rampUp int, autoRampUp bool) (*models.SalesBlinkLink, *errx.Error) {
	l, xerr := s.link(ctx)
	if xerr != nil {
		return nil, xerr
	}
	for name, v := range map[string]int{"warmup_per_day": perDay, "max_daily_frequency": maxDaily, "ramp_up_frequency": rampUp} {
		if v < models.SalesBlinkMinWarmupPerDay || v > models.SalesBlinkMaxWarmupPerDay {
			return nil, errx.NewWithIdentifier(errx.BadRequest, "salesblink_volume_range",
				fmt.Sprintf("%s must be between %d and %d — SalesBlink caps warmup volume per mailbox.",
					name, models.SalesBlinkMinWarmupPerDay, models.SalesBlinkMaxWarmupPerDay))
		}
	}
	l.AutoEnroll, l.WarmupPerDay, l.MaxDailyFrequency = autoEnroll, perDay, maxDaily
	l.RampUpFrequency, l.AutoRampUp = rampUp, autoRampUp
	if err := s.repo.Put(ctx, l); err != nil {
		return nil, errx.InternalError()
	}
	return l, nil
}

func (s *service) Disconnect(ctx context.Context) *errx.Error {
	// Local rows only. The senders stay in SalesBlink: deleting someone's
	// mailboxes out of their SalesBlink account on a disconnect here would be
	// far more destructive than leaving them.
	if err := s.repo.UnenrollAll(ctx); err != nil {
		return errx.InternalError()
	}
	if err := s.repo.Delete(ctx); err != nil {
		return errx.InternalError()
	}
	return nil
}

// ─── eligibility ─────────────────────────────────────────────────────────

// eligible reports whether a mailbox can be handed to SalesBlink at all.
func eligible(a *models.Email) (bool, string) {
	if a.Status != "active" {
		return false, "Mailbox is not active."
	}
	if models.InboxProvider(a.Provider) != models.InboxProviderSMTPIMAP {
		return false, "Connected with Google/Microsoft sign-in; SalesBlink needs SMTP and IMAP credentials."
	}
	return true, ""
}

// ─── enrollment ──────────────────────────────────────────────────────────

func (s *service) Enroll(ctx context.Context, orgID, accountID uuid.UUID) (*models.SalesBlinkMailboxRow, *errx.Error) {
	l, xerr := s.link(ctx)
	if xerr != nil {
		return nil, xerr
	}
	acc, xerr := s.emails.GetByID(ctx, accountID)
	if xerr != nil {
		return nil, xerr
	}
	if acc.OrganizationID == nil || *acc.OrganizationID != orgID {
		return nil, errx.ErrNotFound
	}
	if _, err := s.enroll(ctx, l, acc); err != nil {
		return nil, err
	}
	return s.row(ctx, acc)
}

// enroll pushes one mailbox to SalesBlink and records the mapping.
func (s *service) enroll(ctx context.Context, l *models.SalesBlinkLink, acc *models.Email) (string, *errx.Error) {
	if acc.Status != "active" {
		return "", ErrMailboxInactive
	}
	if models.InboxProvider(acc.Provider) != models.InboxProviderSMTPIMAP {
		return "", ErrOAuthMailbox
	}
	creds, xerr := s.emails.GetSMTPCredentials(ctx, acc.ID)
	if xerr != nil {
		return "", xerr
	}
	// Warmup is a no-op on an SMTP-only sender in SalesBlink, so refuse here
	// rather than create a sender that silently never warms.
	if strings.TrimSpace(creds.IMAPHost) == "" {
		return "", ErrNoIMAP
	}

	req := models.SalesBlinkSenderRequest{
		Email:      acc.Email,
		SenderName: acc.Name,

		SMTPHost:     creds.SMTPHost,
		SMTPPort:     creds.SMTPPort,
		SMTPUsername: creds.SMTPUser,
		SMTPPassword: creds.SMTPPassword,

		IMAPHost:     creds.IMAPHost,
		IMAPPort:     creds.IMAPPort,
		IMAPUsername: creds.IMAPUser,
		IMAPPassword: creds.IMAPPassword,

		WarmupEnabled:     true,
		InboxEnable:       true,
		TotalWarmupPerDay: l.WarmupPerDay,
		AutoRampUpEnabled: l.AutoRampUp,
		RampUpFrequency:   l.RampUpFrequency,
		MaxDailyFrequency: l.MaxDailyFrequency,
	}

	var res models.SalesBlinkSenderResponse
	if xerr := s.clientFor(l).do(ctx, http.MethodPost, "/senders", req, &res); xerr != nil {
		return "", xerr
	}
	senderID := res.ResolveID()
	if senderID == "" {
		return "", errx.NewWithIdentifier(errx.ServiceUnavailable, "salesblink_no_sender_id",
			"SalesBlink accepted the mailbox but did not return a sender id.")
	}
	if _, err := s.repo.Enroll(ctx, acc.ID, senderID); err != nil {
		return "", errx.InternalError()
	}
	return senderID, nil
}

func (s *service) Unenroll(ctx context.Context, orgID, accountID uuid.UUID) *errx.Error {
	acc, xerr := s.emails.GetByID(ctx, accountID)
	if xerr != nil {
		return xerr
	}
	if acc.OrganizationID == nil || *acc.OrganizationID != orgID {
		return errx.ErrNotFound
	}
	m, err := s.repo.GetByAccount(ctx, accountID)
	if err != nil {
		return errx.InternalError()
	}
	if m == nil {
		return nil
	}
	// Turn warmup off in SalesBlink rather than deleting the sender: the
	// mailbox may be in use there for sending, and deleting is not ours to do.
	if l, xerr := s.link(ctx); xerr == nil {
		body := map[string]any{"warmup_enabled": false}
		if xerr := s.clientFor(l).do(ctx, http.MethodPatch, "/senders/"+m.SenderID, body, nil); xerr != nil {
			log.Warn().Str("account_id", accountID.String()).Str("sender_id", m.SenderID).
				Str("code", xerr.Identifier).Msg("salesblink: could not disable warmup remotely; removing local enrollment anyway")
		}
	}
	if err := s.repo.Unenroll(ctx, accountID); err != nil {
		return errx.InternalError()
	}
	return nil
}

// ─── bulk ────────────────────────────────────────────────────────────────

func (s *service) SyncAll(ctx context.Context, orgID uuid.UUID) (*models.SalesBlinkSyncResult, *errx.Error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	l, xerr := s.link(ctx)
	if xerr != nil {
		return nil, xerr
	}
	accounts, xerr := s.emails.GetAllActiveInScope(ctx, repository.NewAccountScope(&orgID))
	if xerr != nil {
		return nil, xerr
	}
	enrolled, err := s.repo.List(ctx)
	if err != nil {
		return nil, errx.InternalError()
	}
	already := make(map[uuid.UUID]struct{}, len(enrolled))
	for _, e := range enrolled {
		already[e.EmailAccountID] = struct{}{}
	}

	out := &models.SalesBlinkSyncResult{}
	for i := range accounts {
		acc := &accounts[i]
		if _, done := already[acc.ID]; done {
			continue
		}
		if ok, _ := eligible(acc); !ok {
			out.Skipped++
			continue
		}
		if _, xerr := s.enroll(ctx, l, acc); xerr != nil {
			out.Failed++
			// Cap the error list; with 200 mailboxes a systemic failure would
			// otherwise return 200 copies of the same message.
			if len(out.Errors) < 10 {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %s", acc.Email, xerr.Message))
			}
			log.Warn().Str("account_id", acc.ID.String()).Str("code", xerr.Identifier).
				Msg("salesblink: enrollment failed during sync")
			continue
		}
		out.Enrolled++
	}

	lastErr := ""
	if out.Failed > 0 {
		lastErr = fmt.Sprintf("%d of %d mailboxes could not be enrolled", out.Failed, out.Failed+out.Enrolled)
	}
	if err := s.repo.SetSyncResult(ctx, time.Now(), lastErr); err != nil {
		log.Warn().Err(err).Msg("salesblink: could not record sync result")
	}
	return out, nil
}

func (s *service) AutoEnroll(ctx context.Context, accountID uuid.UUID) {
	l, xerr := s.link(ctx)
	if xerr != nil || !l.AutoEnroll {
		return
	}
	acc, xerr := s.emails.GetByID(ctx, accountID)
	if xerr != nil {
		return
	}
	if ok, _ := eligible(acc); !ok {
		return
	}
	if enrolled, err := s.repo.IsEnrolled(ctx, accountID); err != nil || enrolled {
		return
	}
	if _, xerr := s.enroll(ctx, l, acc); xerr != nil {
		log.Warn().Str("account_id", accountID.String()).Str("code", xerr.Identifier).
			Msg("salesblink: auto-enroll failed; enroll it from Settings > SalesBlink")
		return
	}
	log.Info().Str("account_id", accountID.String()).Str("email", acc.Email).
		Msg("salesblink: mailbox auto-enrolled in warmup")
}

func (s *service) RefreshHealth(ctx context.Context) error {
	l, xerr := s.link(ctx)
	if xerr != nil {
		return nil // not connected: nothing to do
	}
	rows, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	c := s.clientFor(l)
	for _, m := range rows {
		var h models.SalesBlinkHealth
		if xerr := c.do(ctx, http.MethodGet, "/senders/"+m.SenderID+"/health", nil, &h); xerr != nil {
			_ = s.repo.SetHealth(ctx, m.EmailAccountID, nil, xerr.Message)
			continue
		}
		score := h.Score
		_ = s.repo.SetHealth(ctx, m.EmailAccountID, &score, "")
	}
	return nil
}

// ─── reads ───────────────────────────────────────────────────────────────

func (s *service) Status(ctx context.Context, orgID uuid.UUID) (*models.SalesBlinkStatus, *errx.Error) {
	l, err := s.repo.Get(ctx)
	if err != nil {
		return nil, errx.InternalError()
	}
	st := &models.SalesBlinkStatus{Connected: l != nil, Link: l}
	rows, xerr := s.ListMailboxes(ctx, orgID)
	if xerr != nil {
		return st, nil
	}
	for _, r := range rows {
		switch {
		case r.Enrolled:
			st.Enrolled++
		case r.Eligible:
			st.Eligible++
		default:
			st.Ineligible++
		}
	}
	return st, nil
}

func (s *service) ListMailboxes(ctx context.Context, orgID uuid.UUID) ([]models.SalesBlinkMailboxRow, *errx.Error) {
	accounts, xerr := s.emails.GetAllActiveInScope(ctx, repository.NewAccountScope(&orgID))
	if xerr != nil {
		return nil, xerr
	}
	enrolled, err := s.repo.List(ctx)
	if err != nil {
		return nil, errx.InternalError()
	}
	byAccount := make(map[uuid.UUID]models.SalesBlinkMailbox, len(enrolled))
	for _, e := range enrolled {
		byAccount[e.EmailAccountID] = e
	}

	out := make([]models.SalesBlinkMailboxRow, 0, len(accounts))
	for i := range accounts {
		a := &accounts[i]
		ok, reason := eligible(a)
		row := models.SalesBlinkMailboxRow{
			EmailAccountID: a.ID, Email: a.Email, Provider: a.Provider,
			Status: a.Status, Eligible: ok, IneligibleReason: reason,
		}
		if m, found := byAccount[a.ID]; found {
			row.Enrolled = true
			row.SenderID = m.SenderID
			row.HealthScore = m.HealthScore
			row.EnrolledAt = &m.EnrolledAt
			row.LastError = m.LastError
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *service) row(ctx context.Context, acc *models.Email) (*models.SalesBlinkMailboxRow, *errx.Error) {
	ok, reason := eligible(acc)
	row := &models.SalesBlinkMailboxRow{
		EmailAccountID: acc.ID, Email: acc.Email, Provider: acc.Provider,
		Status: acc.Status, Eligible: ok, IneligibleReason: reason,
	}
	m, err := s.repo.GetByAccount(ctx, acc.ID)
	if err != nil {
		return nil, errx.InternalError()
	}
	if m != nil {
		row.Enrolled = true
		row.SenderID = m.SenderID
		row.HealthScore = m.HealthScore
		row.EnrolledAt = &m.EnrolledAt
		row.LastError = m.LastError
	}
	return row, nil
}

func (s *service) IsEnrolled(ctx context.Context, accountID uuid.UUID) bool {
	ok, err := s.repo.IsEnrolled(ctx, accountID)
	if err != nil {
		return false
	}
	return ok
}
