package repository

import (
	"context"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/utils/paging"
	"github.com/google/uuid"
)

// suppressedHashMatch is an SQL predicate: true when emailExpr's md5 or sha256
// digest sits on one of orgExpr's hashed suppression lists.
func suppressedHashMatch(orgExpr, emailExpr string) string {
	norm := "lower(trim(" + emailExpr + "))"
	return `EXISTS (
		SELECT 1 FROM suppressed_hashes sh
		WHERE sh.organization_id = ` + orgExpr + `
		  AND ((sh.algo = 'md5' AND sh.hash = md5(` + norm + `))
		    OR (sh.algo = 'sha256' AND sh.hash = encode(sha256(convert_to(` + norm + `, 'UTF8')), 'hex')))
	)`
}

const suppressedRecipientColumns = `id, organization_id, email, reason, source, campaign_id, expires_at, metadata, created_at, updated_at`

func (r *advancedOutreachRepository) findHashedSuppression(ctx context.Context, organizationID uuid.UUID, email string) (*models.SuppressedRecipient, error) {
	md5Hex, shaHex := models.SuppressionDigests(email)
	var listName string
	var createdAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT list_name, created_at FROM suppressed_hashes
		WHERE organization_id = $1
		  AND ((algo = 'md5' AND hash = $2) OR (algo = 'sha256' AND hash = $3))
		LIMIT 1
	`, organizationID, md5Hex, shaHex).Scan(&listName, &createdAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	reason := "hashed suppression list"
	if listName != "" {
		reason += ": " + listName
	}
	return &models.SuppressedRecipient{
		OrganizationID: organizationID,
		Email:          models.NormalizeSuppressionEmail(email),
		Reason:         reason,
		Source:         models.SuppressionSourceHashed,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}, nil
}

func (r *advancedOutreachRepository) ListSuppressions(ctx context.Context, organizationID uuid.UUID, query, source string, limit int, cursor *uuid.UUID) (*models.SuppressionListResult, error) {
	where := `organization_id = $1
		AND ($2 = '' OR email ILIKE '%' || $2 || '%' OR reason ILIKE '%' || $2 || '%')
		AND ($3 = '' OR source = $3)`
	args := []any{organizationID, strings.TrimSpace(query), strings.TrimSpace(source)}

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM suppressed_recipients WHERE `+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+suppressedRecipientColumns+` FROM suppressed_recipients
		WHERE `+where+`
		  AND ($4::uuid IS NULL OR (created_at, id) < (
			SELECT created_at, id FROM suppressed_recipients WHERE id = $4 AND organization_id = $1
		  ))
		ORDER BY created_at DESC, id DESC
		LIMIT $5
	`, append(args, cursor, limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.SuppressedRecipient, 0, limit)
	for rows.Next() {
		entry, err := scanSuppressedRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := &models.SuppressionListResult{Pagination: models.Pagination{Total: &total}}
	if len(out) > limit {
		res.Pagination.HasMore = true
		res.Pagination.NextCursor = paging.EncodeUUID(out[limit-1].ID)
		out = out[:limit]
	}
	res.Data = out
	return res, nil
}

// BulkInsertSuppressedRecipients adds addresses that are not already suppressed
// and reports how many were new. Existing entries keep their original reason.
func (r *advancedOutreachRepository) BulkInsertSuppressedRecipients(ctx context.Context, organizationID uuid.UUID, emails []string, reason string, source models.DeliverabilityEventType, expiresAt *time.Time) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO suppressed_recipients (organization_id, email, reason, source, expires_at, metadata, created_at, updated_at)
		SELECT $1, e, $3, $4, $5, '{}'::jsonb, NOW(), NOW()
		FROM (SELECT DISTINCT lower(trim(x)) AS e FROM unnest($2::text[]) AS x) s
		WHERE e <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM suppressed_recipients sr
			WHERE sr.organization_id = $1 AND lower(sr.email) = s.e
		  )
		ON CONFLICT (organization_id, email) DO NOTHING
	`, organizationID, emails, reason, string(source), expiresAt)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *advancedOutreachRepository) DeleteSuppressedRecipient(ctx context.Context, organizationID, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM suppressed_recipients WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *advancedOutreachRepository) DeleteSuppressedEmails(ctx context.Context, organizationID uuid.UUID, emails []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `
		DELETE FROM suppressed_recipients
		WHERE organization_id = $1
		  AND lower(email) IN (SELECT lower(trim(x)) FROM unnest($2::text[]) AS x)
	`, organizationID, emails)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// InsertSuppressedHashes adds lowercase hex digests to a named list and reports how many were new.
func (r *advancedOutreachRepository) InsertSuppressedHashes(ctx context.Context, organizationID uuid.UUID, algo models.SuppressionHashAlgo, listName string, hashes []string) (int, error) {
	if len(hashes) == 0 {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO suppressed_hashes (organization_id, algo, hash, list_name)
		SELECT DISTINCT $1::uuid, $2, h, $3 FROM unnest($4::text[]) AS h
		ON CONFLICT (organization_id, algo, hash) DO NOTHING
	`, organizationID, string(algo), listName, hashes)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *advancedOutreachRepository) ListSuppressionHashLists(ctx context.Context, organizationID uuid.UUID) ([]models.SuppressionHashList, error) {
	rows, err := r.db.Query(ctx, `
		SELECT list_name, algo, COUNT(*), MIN(created_at)
		FROM suppressed_hashes
		WHERE organization_id = $1
		GROUP BY list_name, algo
		ORDER BY MIN(created_at) DESC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SuppressionHashList{}
	for rows.Next() {
		var l models.SuppressionHashList
		if err := rows.Scan(&l.ListName, &l.Algo, &l.Count, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *advancedOutreachRepository) DeleteSuppressionHashList(ctx context.Context, organizationID uuid.UUID, algo models.SuppressionHashAlgo, listName string) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM suppressed_hashes WHERE organization_id = $1 AND algo = $2 AND list_name = $3`, organizationID, string(algo), listName)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// EachSuppressedEmail streams every active plaintext suppression in creation order.
func (r *advancedOutreachRepository) EachSuppressedEmail(ctx context.Context, organizationID uuid.UUID, fn func(entry *models.SuppressedRecipient) error) error {
	rows, err := r.db.Query(ctx, `
		SELECT `+suppressedRecipientColumns+` FROM suppressed_recipients
		WHERE organization_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at, id
	`, organizationID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		entry, err := scanSuppressedRecipient(rows)
		if err != nil {
			return err
		}
		if err := fn(entry); err != nil {
			return err
		}
	}
	return rows.Err()
}
