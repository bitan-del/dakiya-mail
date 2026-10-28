package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const brandColumns = `b.id, b.organization_id, b.name, b.website, b.physical_address, b.footer_text, b.footer_enabled, b.unsubscribe_link, b.created_at, b.updated_at,
	COALESCE((SELECT array_agg(ea.id ORDER BY ea.email) FROM email_accounts ea WHERE ea.brand_id = b.id), '{}')`

func scanBrand(row pgx.Row) (*models.Brand, error) {
	var b models.Brand
	if err := row.Scan(&b.ID, &b.OrganizationID, &b.Name, &b.Website, &b.PhysicalAddress, &b.FooterText, &b.FooterEnabled, &b.UnsubscribeLink, &b.CreatedAt, &b.UpdatedAt, &b.MailboxIDs); err != nil {
		return nil, err
	}
	return &b, nil
}

func brandWriteErr(err error, where string) *errx.Error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errx.NewWithIdentifier(errx.Conflict, "brand_name_taken", "a brand with that name already exists")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errx.New(errx.NotFound, "brand not found")
	}
	db.CaptureError(err, "", nil, where)
	return errx.InternalError()
}

func (r *emailRepository) ListBrands(ctx context.Context, orgID uuid.UUID) ([]models.Brand, *errx.Error) {
	rows, err := r.DB.Query(ctx, `SELECT `+brandColumns+` FROM brands b WHERE b.organization_id = $1 ORDER BY lower(b.name)`, orgID)
	if err != nil {
		return nil, brandWriteErr(err, "list brands")
	}
	defer rows.Close()
	out := []models.Brand{}
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, brandWriteErr(err, "scan brand")
		}
		out = append(out, *b)
	}
	return out, nil
}

func (r *emailRepository) GetBrand(ctx context.Context, orgID, id uuid.UUID) (*models.Brand, *errx.Error) {
	b, err := scanBrand(r.DB.QueryRow(ctx, `SELECT `+brandColumns+` FROM brands b WHERE b.organization_id = $1 AND b.id = $2`, orgID, id))
	if err != nil {
		return nil, brandWriteErr(err, "get brand")
	}
	return b, nil
}

func (r *emailRepository) SaveBrand(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, req *models.SaveBrandRequest) (uuid.UUID, *errx.Error) {
	footer, unsub := true, true
	if req.FooterEnabled != nil {
		footer = *req.FooterEnabled
	}
	if req.UnsubscribeLink != nil {
		unsub = *req.UnsubscribeLink
	}
	args := []any{orgID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Website), strings.TrimSpace(req.PhysicalAddress), strings.TrimSpace(req.FooterText), footer, unsub}
	var out uuid.UUID
	var err error
	if id == nil {
		err = r.DB.QueryRow(ctx, `
			INSERT INTO brands (organization_id, name, website, physical_address, footer_text, footer_enabled, unsubscribe_link)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, args...).Scan(&out)
	} else {
		err = r.DB.QueryRow(ctx, `
			UPDATE brands SET name = $2, website = $3, physical_address = $4, footer_text = $5, footer_enabled = $6, unsubscribe_link = $7, updated_at = NOW()
			WHERE organization_id = $1 AND id = $8 RETURNING id`, append(args, *id)...).Scan(&out)
	}
	if err != nil {
		return uuid.Nil, brandWriteErr(err, "save brand")
	}
	return out, nil
}

func (r *emailRepository) DeleteBrand(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM brands WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return brandWriteErr(err, "delete brand")
	}
	if tag.RowsAffected() == 0 {
		return errx.New(errx.NotFound, "brand not found")
	}
	return nil
}

// SetBrandMailboxes makes exactly these org mailboxes send as the brand.
func (r *emailRepository) SetBrandMailboxes(ctx context.Context, orgID, brandID uuid.UUID, accountIDs []uuid.UUID) *errx.Error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return brandWriteErr(err, "begin")
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE email_accounts SET brand_id = NULL WHERE organization_id = $1 AND brand_id = $2 AND NOT (id = ANY($3))`, orgID, brandID, accountIDs); err != nil {
		return brandWriteErr(err, "clear brand mailboxes")
	}
	if _, err := tx.Exec(ctx, `UPDATE email_accounts SET brand_id = $2 WHERE organization_id = $1 AND id = ANY($3)`, orgID, brandID, accountIDs); err != nil {
		return brandWriteErr(err, "set brand mailboxes")
	}
	if err := tx.Commit(ctx); err != nil {
		return brandWriteErr(err, "commit")
	}
	return nil
}

// GetBrandForAccount returns the brand a mailbox sends as, or nil.
func (r *emailRepository) GetBrandForAccount(ctx context.Context, accountID uuid.UUID) (*models.Brand, error) {
	b, err := scanBrand(r.DB.QueryRow(ctx, `SELECT `+brandColumns+` FROM brands b JOIN email_accounts ea ON ea.brand_id = b.id WHERE ea.id = $1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}
