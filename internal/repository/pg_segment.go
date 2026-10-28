package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const segmentColumns = `id, organization_id, name, description, filter, created_by, created_at, updated_at`

func scanSegment(row pgx.Row) (*models.Segment, error) {
	var s models.Segment
	var filter []byte
	if err := row.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Description, &filter, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	if len(filter) > 0 {
		_ = json.Unmarshal(filter, &s.Filter)
	}
	return &s, nil
}

func segmentWriteErr(err error, where string) *errx.Error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errx.NewWithIdentifier(errx.Conflict, "segment_name_taken", "a segment with that name already exists")
	}
	db.CaptureError(err, "", nil, where)
	return errx.InternalError()
}

func (r *contactRepository) CreateSegment(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, req *models.SaveSegmentRequest) (*models.Segment, *errx.Error) {
	filter, err := json.Marshal(req.Filter)
	if err != nil {
		return nil, errx.InternalError()
	}
	s, err := scanSegment(r.DB.QueryRow(ctx, `
		INSERT INTO contact_segments (organization_id, name, description, filter, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+segmentColumns, orgID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), filter, userID))
	if err != nil {
		return nil, segmentWriteErr(err, "create segment")
	}
	return s, nil
}

func (r *contactRepository) UpdateSegment(ctx context.Context, orgID, id uuid.UUID, req *models.SaveSegmentRequest) (*models.Segment, *errx.Error) {
	filter, err := json.Marshal(req.Filter)
	if err != nil {
		return nil, errx.InternalError()
	}
	s, err := scanSegment(r.DB.QueryRow(ctx, `
		UPDATE contact_segments SET name = $3, description = $4, filter = $5, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+segmentColumns, orgID, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), filter))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.New(errx.NotFound, "segment not found")
		}
		return nil, segmentWriteErr(err, "update segment")
	}
	return s, nil
}

func (r *contactRepository) GetSegment(ctx context.Context, orgID, id uuid.UUID) (*models.Segment, *errx.Error) {
	s, err := scanSegment(r.DB.QueryRow(ctx, `SELECT `+segmentColumns+` FROM contact_segments WHERE organization_id = $1 AND id = $2`, orgID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.New(errx.NotFound, "segment not found")
		}
		db.CaptureError(err, "", nil, "get segment")
		return nil, errx.InternalError()
	}
	return s, nil
}

func (r *contactRepository) ListSegments(ctx context.Context, orgID uuid.UUID) ([]models.Segment, *errx.Error) {
	rows, err := r.DB.Query(ctx, `SELECT `+segmentColumns+` FROM contact_segments WHERE organization_id = $1 ORDER BY lower(name)`, orgID)
	if err != nil {
		db.CaptureError(err, "", nil, "list segments")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.Segment{}
	for rows.Next() {
		s, err := scanSegment(rows)
		if err != nil {
			db.CaptureError(err, "", nil, "scan segment")
			return nil, errx.InternalError()
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *contactRepository) DeleteSegment(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM contact_segments WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		db.CaptureError(err, "", nil, "delete segment")
		return errx.InternalError()
	}
	if tag.RowsAffected() == 0 {
		return errx.New(errx.NotFound, "segment not found")
	}
	return nil
}

// CountCampaignLeads reports how many of the given contacts are already leads of the campaign.
func (r *contactRepository) CountCampaignLeads(ctx context.Context, campaignID uuid.UUID, contactIDs []uuid.UUID) (int, *errx.Error) {
	var n int
	if err := r.DB.QueryRow(ctx, `SELECT COUNT(*) FROM campaign_leads WHERE campaign_id = $1 AND contact_id = ANY($2)`, campaignID, contactIDs).Scan(&n); err != nil {
		db.CaptureError(err, "", nil, "count campaign leads")
		return 0, errx.InternalError()
	}
	return n, nil
}

func (r *contactRepository) CampaignInOrganization(ctx context.Context, orgID, campaignID uuid.UUID) (bool, *errx.Error) {
	var ok bool
	if err := r.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1 AND organization_id = $2)`, campaignID, orgID).Scan(&ok); err != nil {
		db.CaptureError(err, "", nil, "campaign in organization")
		return false, errx.InternalError()
	}
	return ok, nil
}

// RecentContactEmails returns which of the lowercased addresses the organization added at or after since.
func (r *contactRepository) RecentContactEmails(ctx context.Context, orgID uuid.UUID, emails []string, since time.Time) (map[string]bool, *errx.Error) {
	out := map[string]bool{}
	if len(emails) == 0 {
		return out, nil
	}
	rows, err := r.DB.Query(ctx, `
		SELECT DISTINCT lower(email) FROM contacts
		WHERE organization_id = $1 AND lower(email) = ANY($2) AND first_seen_at >= $3
	`, orgID, emails, since)
	if err != nil {
		db.CaptureError(err, "", nil, "recent contact emails")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			db.CaptureError(err, "", nil, "scan recent contact email")
			return nil, errx.InternalError()
		}
		out[e] = true
	}
	return out, nil
}
