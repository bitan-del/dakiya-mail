package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// pgEmailMessageMapRepository is the Postgres-backed implementation used by the
// backend, which serves it to workers over the internal API. The composite key
// (user_id, email_id, message_id) is canonical — see the 000003 migration.
type pgEmailMessageMapRepository struct {
	db *db.DB
}

// NewEmailMessageMapRepository returns the Postgres-backed implementation.
func NewEmailMessageMapRepository(d *db.DB) EmailMessageMapRepository {
	return &pgEmailMessageMapRepository{db: d}
}

func (r *pgEmailMessageMapRepository) Add(ctx context.Context, data EmailMessageData) error {
	userID, err := uuid.Parse(data.UserID)
	if err != nil {
		return fmt.Errorf("email_message_map: invalid user_id: %w", err)
	}
	emailID, err := uuid.Parse(data.EmailID)
	if err != nil {
		return fmt.Errorf("email_message_map: invalid email_id: %w", err)
	}
	id, err := uuid.Parse(data.ID)
	if err != nil {
		return fmt.Errorf("email_message_map: invalid id: %w", err)
	}

	// Upsert: the old DynamoDB PutItem overwrote unconditionally.
	const q = `
		INSERT INTO email_message_map (user_id, email_id, message_id, id, thread_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, email_id, message_id)
		DO UPDATE SET id = EXCLUDED.id, thread_id = EXCLUDED.thread_id
	`
	if _, err := r.db.Exec(ctx, q, userID, emailID, data.MessageID, id, data.ThreadID); err != nil {
		return fmt.Errorf("email_message_map: add: %w", err)
	}
	return nil
}

func (r *pgEmailMessageMapRepository) Get(ctx context.Context, userID, emailID uuid.UUID, messageID string) (*EmailMessageData, error) {
	const q = `
		SELECT id, thread_id
		FROM email_message_map
		WHERE user_id = $1 AND email_id = $2 AND message_id = $3
	`
	var id uuid.UUID
	var threadID string
	err := r.db.QueryRow(ctx, q, userID, emailID, messageID).Scan(&id, &threadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("email_message_map: get: %w", err)
	}
	return &EmailMessageData{
		UserID:    userID.String(),
		EmailID:   emailID.String(),
		MessageID: messageID,
		ID:        id.String(),
		ThreadID:  threadID,
	}, nil
}

func (r *pgEmailMessageMapRepository) Del(ctx context.Context, userID, emailID uuid.UUID, messageID string, _ uuid.UUID) error {
	// (user_id, email_id, message_id) is the primary key, so the internal id
	// is not needed to identify the row.
	const q = `DELETE FROM email_message_map WHERE user_id = $1 AND email_id = $2 AND message_id = $3`
	if _, err := r.db.Exec(ctx, q, userID, emailID, messageID); err != nil {
		return fmt.Errorf("email_message_map: del: %w", err)
	}
	return nil
}
