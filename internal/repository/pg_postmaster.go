package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/encrypt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPostmasterNoEncrypter means credentials cannot be sealed, so they are never stored.
var ErrPostmasterNoEncrypter = errors.New("credential encrypter not configured (set CREDENTIALS_ENCRYPTION_KEY)")

// PostmasterRepository holds the Google Postmaster credentials (instance-wide,
// sealed) and the per-workspace daily stats pulled with them.
type PostmasterRepository interface {
	// GetConnection returns nil when no credentials are stored.
	GetConnection(ctx context.Context) (*models.PostmasterConnection, error)
	PutCredentials(ctx context.Context, creds models.PostmasterCredentials) error
	DeleteCredentials(ctx context.Context) error
	SetSyncResult(ctx context.Context, at time.Time, lastError string) error

	// MailboxDomains lists every workspace's sending domains, consumer providers excluded.
	MailboxDomains(ctx context.Context) ([]models.PostmasterOrgDomain, error)
	// OrgDomains lists one workspace's sending domains with their mailbox counts and newest stored day.
	OrgDomains(ctx context.Context, orgID uuid.UUID) ([]models.PostmasterDomain, error)
	UpsertStats(ctx context.Context, orgID uuid.UUID, domain string, stats []models.PostmasterStat) error
	ListStats(ctx context.Context, orgID uuid.UUID, domain string, days int) ([]models.PostmasterDay, error)
}

type postmasterRepository struct {
	db      *pgxpool.Pool
	encrypt *encrypt.Encrypter
}

// NewPostmasterRepository seals the Google credentials with the mailbox credential key.
func NewPostmasterRepository(db *pgxpool.Pool, enc *encrypt.Encrypter) PostmasterRepository {
	return &postmasterRepository{db: db, encrypt: enc}
}

func (r *postmasterRepository) GetConnection(ctx context.Context) (*models.PostmasterConnection, error) {
	query := `SELECT postmaster_credentials, postmaster_synced_at, postmaster_last_error FROM instance_settings WHERE id = true`
	var sealed string
	var c models.PostmasterConnection
	if err := r.db.QueryRow(ctx, query).Scan(&sealed, &c.SyncedAt, &c.LastError); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		db.CaptureError(err, query, nil, "queryrow")
		return nil, err
	}
	if sealed == "" {
		return nil, nil
	}
	if r.encrypt == nil {
		return nil, ErrPostmasterNoEncrypter
	}
	plain, err := r.encrypt.Decrypt(sealed)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(plain), &c.Credentials); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *postmasterRepository) PutCredentials(ctx context.Context, creds models.PostmasterCredentials) error {
	if r.encrypt == nil {
		return ErrPostmasterNoEncrypter
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	sealed, err := r.encrypt.Encrypt(string(raw))
	if err != nil {
		return err
	}
	// The settings row is seeded by migration 000083; the upsert only covers a hand-deleted one.
	query := `
		INSERT INTO instance_settings (id, postmaster_credentials) VALUES (true, $1)
		ON CONFLICT (id) DO UPDATE SET postmaster_credentials = EXCLUDED.postmaster_credentials,
		  postmaster_synced_at = NULL, postmaster_last_error = ''
	`
	if _, err := r.db.Exec(ctx, query, sealed); err != nil {
		db.CaptureError(err, query, nil, "exec")
		return err
	}
	return nil
}

func (r *postmasterRepository) DeleteCredentials(ctx context.Context) error {
	query := `UPDATE instance_settings SET postmaster_credentials = '', postmaster_synced_at = NULL, postmaster_last_error = '' WHERE id = true`
	if _, err := r.db.Exec(ctx, query); err != nil {
		db.CaptureError(err, query, nil, "exec")
		return err
	}
	return nil
}

func (r *postmasterRepository) SetSyncResult(ctx context.Context, at time.Time, lastError string) error {
	query := `UPDATE instance_settings SET postmaster_synced_at = CASE WHEN $2 = '' THEN $1 ELSE postmaster_synced_at END, postmaster_last_error = $2 WHERE id = true`
	_, err := r.db.Exec(ctx, query, at, lastError)
	return err
}

func (r *postmasterRepository) MailboxDomains(ctx context.Context) ([]models.PostmasterOrgDomain, error) {
	query := `
		SELECT DISTINCT organization_id, lower(split_part(email, '@', 2))
		FROM email_accounts
		WHERE organization_id IS NOT NULL AND position('@' in email) > 0
		  AND lower(split_part(email, '@', 2)) NOT IN (` + freeMailDomainsSQL + `)
		  AND split_part(email, '@', 2) <> ''
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		db.CaptureError(err, query, nil, "query")
		return nil, err
	}
	defer rows.Close()
	var out []models.PostmasterOrgDomain
	for rows.Next() {
		var d models.PostmasterOrgDomain
		if err := rows.Scan(&d.OrganizationID, &d.Domain); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *postmasterRepository) OrgDomains(ctx context.Context, orgID uuid.UUID) ([]models.PostmasterDomain, error) {
	query := `
		SELECT d.domain, d.mailboxes,
		       (SELECT max(s.date) FROM postmaster_domain_stats s WHERE s.organization_id = $1 AND s.domain = d.domain)
		FROM (
			SELECT lower(split_part(email, '@', 2)) AS domain, count(*)::int AS mailboxes
			FROM email_accounts
			WHERE organization_id = $1 AND position('@' in email) > 0
			GROUP BY 1
		) d
		WHERE d.domain <> '' AND d.domain NOT IN (` + freeMailDomainsSQL + `)
		ORDER BY d.mailboxes DESC, d.domain
	`
	rows, err := r.db.Query(ctx, query, orgID)
	if err != nil {
		db.CaptureError(err, query, nil, "query")
		return nil, err
	}
	defer rows.Close()
	out := []models.PostmasterDomain{}
	for rows.Next() {
		var d models.PostmasterDomain
		var latest *time.Time
		if err := rows.Scan(&d.Domain, &d.Mailboxes, &latest); err != nil {
			return nil, err
		}
		d.LatestDate = dateString(latest)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *postmasterRepository) UpsertStats(ctx context.Context, orgID uuid.UUID, domain string, stats []models.PostmasterStat) error {
	if len(stats) == 0 {
		return nil
	}
	// Reputation is COALESCEd: once v1 is retired a refresh must not blank the days it already stored.
	query := `
		INSERT INTO postmaster_domain_stats (organization_id, domain, date, domain_reputation, spam_rate, spf_ratio, dkim_ratio, dmarc_ratio, raw, synced_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9::jsonb, NOW())
		ON CONFLICT (organization_id, domain, date) DO UPDATE SET
		  domain_reputation = COALESCE(EXCLUDED.domain_reputation, postmaster_domain_stats.domain_reputation),
		  spam_rate = EXCLUDED.spam_rate, spf_ratio = EXCLUDED.spf_ratio, dkim_ratio = EXCLUDED.dkim_ratio,
		  dmarc_ratio = EXCLUDED.dmarc_ratio, raw = EXCLUDED.raw, synced_at = NOW()
	`
	batch := &pgx.Batch{}
	for _, s := range stats {
		raw := s.Raw
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		batch.Queue(query, orgID, domain, s.Date, s.DomainReputation, s.SpamRate, s.SPFRatio, s.DKIMRatio, s.DMARCRatio, []byte(raw))
	}
	br := r.db.SendBatch(ctx, batch)
	defer br.Close()
	for range stats {
		if _, err := br.Exec(); err != nil {
			db.CaptureError(err, query, nil, "batch")
			return err
		}
	}
	return nil
}

func (r *postmasterRepository) ListStats(ctx context.Context, orgID uuid.UUID, domain string, days int) ([]models.PostmasterDay, error) {
	query := `
		SELECT date, domain_reputation, spam_rate, spf_ratio, dkim_ratio, dmarc_ratio
		FROM postmaster_domain_stats
		WHERE organization_id = $1 AND domain = $2 AND date > CURRENT_DATE - $3::int
		ORDER BY date
	`
	rows, err := r.db.Query(ctx, query, orgID, domain, days)
	if err != nil {
		db.CaptureError(err, query, nil, "query")
		return nil, err
	}
	defer rows.Close()
	out := []models.PostmasterDay{}
	for rows.Next() {
		var d models.PostmasterDay
		var day time.Time
		if err := rows.Scan(&day, &d.DomainReputation, &d.SpamRate, &d.SPFRatio, &d.DKIMRatio, &d.DMARCRatio); err != nil {
			return nil, err
		}
		d.Date = day.Format("2006-01-02")
		out = append(out, d)
	}
	return out, rows.Err()
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}
