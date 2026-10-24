package repository

import (
	"context"
	"errors"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/pkg/encrypt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Who sent a warmup thread message: the mailbox that opened the thread or the
// one it was addressed to.
const (
	WarmupThreadInitiator = "initiator"
	WarmupThreadResponder = "responder"
)

// WarmupThreadDirection is the direction of a message at a turn: an opening and
// every second turn after it come from the initiator.
func WarmupThreadDirection(turn int) string {
	if turn%2 == 0 {
		return WarmupThreadInitiator
	}
	return WarmupThreadResponder
}

// WarmupThreadMessage is one sent warmup message, in plaintext at this layer.
type WarmupThreadMessage struct {
	ThreadID           uuid.UUID
	Turn               int
	Direction          string
	SenderAccountID    uuid.UUID
	RecipientAccountID uuid.UUID
	Subject            string
	Body               string
	CreatedAt          time.Time
}

// WarmupThreadRepository keeps the text of live-written warmup mail. Rows are
// sealed with the instance credential key because a thread crosses workspaces.
type WarmupThreadRepository interface {
	// Save is idempotent per (thread, turn, direction): a retried send keeps
	// the first text rather than failing.
	Save(ctx context.Context, m *WarmupThreadMessage) error
	// Thread returns up to limit messages of a thread at or before uptoTurn,
	// oldest first.
	Thread(ctx context.Context, threadID uuid.UUID, uptoTurn, limit int) ([]WarmupThreadMessage, error)
	// RecentSentBodies returns the bodies a mailbox sent most recently, newest first.
	RecentSentBodies(ctx context.Context, accountID uuid.UUID, limit int) ([]string, error)
	// PurgeBefore deletes messages older than the cutoff.
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
}

type warmupThreadRepository struct {
	db      *pgxpool.Pool
	encrypt *encrypt.Encrypter
}

var errNoThreadEncrypter = errors.New("credential encrypter not configured (set CREDENTIALS_ENCRYPTION_KEY)")

// NewWarmupThreadRepository seals message text with the instance credential key.
func NewWarmupThreadRepository(db *pgxpool.Pool, enc *encrypt.Encrypter) WarmupThreadRepository {
	return &warmupThreadRepository{db: db, encrypt: enc}
}

func (r *warmupThreadRepository) Save(ctx context.Context, m *WarmupThreadMessage) error {
	if r.encrypt == nil {
		return errNoThreadEncrypter
	}
	subject, err := r.encrypt.Encrypt(m.Subject)
	if err != nil {
		return err
	}
	body, err := r.encrypt.Encrypt(m.Body)
	if err != nil {
		return err
	}
	direction := m.Direction
	if direction == "" {
		direction = WarmupThreadDirection(m.Turn)
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO warmup_thread_messages
			(thread_id, turn, direction, sender_account_id, recipient_account_id, subject_enc, body_enc)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (thread_id, turn, direction) DO NOTHING
	`, m.ThreadID, m.Turn, direction, m.SenderAccountID, m.RecipientAccountID, subject, body)
	return err
}

func (r *warmupThreadRepository) Thread(ctx context.Context, threadID uuid.UUID, uptoTurn, limit int) ([]WarmupThreadMessage, error) {
	if r.encrypt == nil {
		return nil, errNoThreadEncrypter
	}
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.Query(ctx, `
		SELECT thread_id, turn, direction, sender_account_id, recipient_account_id, subject_enc, body_enc, created_at
		FROM warmup_thread_messages
		WHERE thread_id = $1 AND turn <= $2
		ORDER BY turn DESC
		LIMIT $3
	`, threadID, uptoTurn, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WarmupThreadMessage
	for rows.Next() {
		var m WarmupThreadMessage
		var subject, body string
		if err := rows.Scan(&m.ThreadID, &m.Turn, &m.Direction, &m.SenderAccountID, &m.RecipientAccountID, &subject, &body, &m.CreatedAt); err != nil {
			return nil, err
		}
		if m.Subject, err = r.encrypt.Decrypt(subject); err != nil {
			return nil, err
		}
		if m.Body, err = r.encrypt.Decrypt(body); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Queried newest first so LIMIT keeps the latest turns; callers read oldest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (r *warmupThreadRepository) RecentSentBodies(ctx context.Context, accountID uuid.UUID, limit int) ([]string, error) {
	if r.encrypt == nil {
		return nil, errNoThreadEncrypter
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT body_enc FROM warmup_thread_messages
		WHERE sender_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var sealed string
		if err := rows.Scan(&sealed); err != nil {
			return nil, err
		}
		body, err := r.encrypt.Decrypt(sealed)
		if err != nil {
			return nil, err
		}
		out = append(out, body)
	}
	return out, rows.Err()
}

func (r *warmupThreadRepository) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM warmup_thread_messages WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
