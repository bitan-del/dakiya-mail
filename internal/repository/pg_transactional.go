package repository

import (
	"context"
	"errors"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *advancedOutreachRepository) RecordTransactionalMessage(ctx context.Context, orgID, taskID uuid.UUID, templateID *uuid.UUID, recipient string, trackIDs []string) error {
	if trackIDs == nil {
		trackIDs = []string{}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO transactional_messages (task_id, organization_id, template_id, recipient, track_ids)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (task_id) DO NOTHING
	`, taskID, orgID, templateID, recipient, trackIDs)
	return err
}

// TransactionalTrackIDs returns the caller's track ids for a transactional send, or nil.
func (r *advancedOutreachRepository) TransactionalTrackIDs(ctx context.Context, taskID uuid.UUID) ([]string, error) {
	var ids []string
	err := r.db.QueryRow(ctx, `SELECT track_ids FROM transactional_messages WHERE task_id = $1`, taskID).Scan(&ids)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return ids, err
}

func (r *advancedOutreachRepository) GetTransactionalMessage(ctx context.Context, orgID, taskID uuid.UUID) (*models.TransactionalMessage, error) {
	var m models.TransactionalMessage
	err := r.db.QueryRow(ctx, `
		SELECT tm.task_id, t.status, tm.recipient, COALESCE(t.message_id, ''), COALESCE(t.provider_message_id, ''),
		       tm.template_id, tm.track_ids, t.scheduled_at, t.completed_at, tm.created_at
		FROM transactional_messages tm
		JOIN tasks t ON t.id = tm.task_id
		WHERE tm.organization_id = $1 AND tm.task_id = $2
	`, orgID, taskID).Scan(&m.TaskID, &m.Status, &m.Recipient, &m.MessageID, &m.ProviderMessageID,
		&m.TemplateID, &m.TrackIDs, &m.ScheduledAt, &m.CompletedAt, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	events, err := r.ListDeliverabilityEvents(ctx, orgID, models.DeliverabilityEventFilter{TaskID: &taskID}, 200, nil)
	if err != nil {
		return nil, err
	}
	m.Events = events.Data
	return &m, nil
}
