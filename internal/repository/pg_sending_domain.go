package repository

import (
	"context"
	"errors"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const sendingDomainColumns = `d.id, d.organization_id, d.domain, d.provider, d.dkim_selectors, d.state, d.spf_found, d.spf_record,
	d.dkim_found, d.dmarc_found, d.dmarc_policy, d.check_summary, d.last_checked_at, d.created_at, d.updated_at,
	(SELECT COUNT(*) FROM email_accounts ea WHERE ea.organization_id = d.organization_id AND lower(split_part(ea.email, '@', 2)) = d.domain)`

func scanSendingDomain(row pgx.Row) (*models.SendingDomain, error) {
	var d models.SendingDomain
	if err := row.Scan(&d.ID, &d.OrganizationID, &d.Domain, &d.Provider, &d.DKIMSelectors, &d.State, &d.SPFFound, &d.SPFRecord,
		&d.DKIMFound, &d.DMARCFound, &d.DMARCPolicy, &d.CheckSummary, &d.LastCheckedAt, &d.CreatedAt, &d.UpdatedAt, &d.MailboxCount); err != nil {
		return nil, err
	}
	if d.DKIMSelectors == nil {
		d.DKIMSelectors = []string{}
	}
	return &d, nil
}

func (r *emailRepository) ListSendingDomains(ctx context.Context, orgID uuid.UUID) ([]models.SendingDomain, *errx.Error) {
	rows, err := r.DB.Query(ctx, `SELECT `+sendingDomainColumns+` FROM sending_domains d WHERE d.organization_id = $1 ORDER BY d.domain`, orgID)
	if err != nil {
		db.CaptureError(err, "", nil, "list sending domains")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.SendingDomain{}
	for rows.Next() {
		d, err := scanSendingDomain(rows)
		if err != nil {
			db.CaptureError(err, "", nil, "scan sending domain")
			return nil, errx.InternalError()
		}
		out = append(out, *d)
	}
	return out, nil
}

func (r *emailRepository) GetSendingDomain(ctx context.Context, orgID, id uuid.UUID) (*models.SendingDomain, *errx.Error) {
	d, err := scanSendingDomain(r.DB.QueryRow(ctx, `SELECT `+sendingDomainColumns+` FROM sending_domains d WHERE d.organization_id = $1 AND d.id = $2`, orgID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.New(errx.NotFound, "domain not found")
		}
		db.CaptureError(err, "", nil, "get sending domain")
		return nil, errx.InternalError()
	}
	return d, nil
}

func (r *emailRepository) CreateSendingDomain(ctx context.Context, orgID uuid.UUID, req *models.SaveSendingDomainRequest) (uuid.UUID, *errx.Error) {
	var id uuid.UUID
	err := r.DB.QueryRow(ctx, `
		INSERT INTO sending_domains (organization_id, domain, provider, dkim_selectors)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, orgID, req.Domain, string(req.Provider), req.DKIMSelectors).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return uuid.Nil, errx.NewWithIdentifier(errx.Conflict, "domain_already_added", "this domain is already added")
		}
		db.CaptureError(err, "", nil, "create sending domain")
		return uuid.Nil, errx.InternalError()
	}
	return id, nil
}

func (r *emailRepository) UpdateSendingDomainSetup(ctx context.Context, orgID, id uuid.UUID, provider models.SendingDomainProvider, selectors []string) *errx.Error {
	tag, err := r.DB.Exec(ctx, `
		UPDATE sending_domains SET provider = $3, dkim_selectors = $4, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, orgID, id, string(provider), selectors)
	if err != nil {
		db.CaptureError(err, "", nil, "update sending domain")
		return errx.InternalError()
	}
	if tag.RowsAffected() == 0 {
		return errx.New(errx.NotFound, "domain not found")
	}
	return nil
}

func (r *emailRepository) RecordSendingDomainCheck(ctx context.Context, id uuid.UUID, state string, spf bool, spfRecord string, dkim, dmarc bool, dmarcPolicy, summary string, checkedAt time.Time) *errx.Error {
	_, err := r.DB.Exec(ctx, `
		UPDATE sending_domains
		SET state = $2, spf_found = $3, spf_record = $4, dkim_found = $5, dmarc_found = $6,
		    dmarc_policy = $7, check_summary = $8, last_checked_at = $9, updated_at = NOW()
		WHERE id = $1
	`, id, state, spf, spfRecord, dkim, dmarc, dmarcPolicy, summary, checkedAt)
	if err != nil {
		db.CaptureError(err, "", nil, "record sending domain check")
		return errx.InternalError()
	}
	return nil
}

func (r *emailRepository) DeleteSendingDomain(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM sending_domains WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		db.CaptureError(err, "", nil, "delete sending domain")
		return errx.InternalError()
	}
	if tag.RowsAffected() == 0 {
		return errx.New(errx.NotFound, "domain not found")
	}
	return nil
}
