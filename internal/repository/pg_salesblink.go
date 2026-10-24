package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/encrypt"
)

// SalesBlinkRepository stores the SalesBlink link and the mailbox enrollments.
type SalesBlinkRepository interface {
	Get(ctx context.Context) (*models.SalesBlinkLink, error)
	Put(ctx context.Context, link *models.SalesBlinkLink) error
	Delete(ctx context.Context) error
	SetSyncResult(ctx context.Context, at time.Time, lastError string) error

	Enroll(ctx context.Context, accountID uuid.UUID, senderID string) (*models.SalesBlinkMailbox, error)
	Unenroll(ctx context.Context, accountID uuid.UUID) error
	UnenrollAll(ctx context.Context) error
	GetByAccount(ctx context.Context, accountID uuid.UUID) (*models.SalesBlinkMailbox, error)
	List(ctx context.Context) ([]models.SalesBlinkMailbox, error)
	IsEnrolled(ctx context.Context, accountID uuid.UUID) (bool, error)
	SetHealth(ctx context.Context, accountID uuid.UUID, score *int, lastError string) error
}

type salesBlinkRepository struct {
	db      *pgxpool.Pool
	encrypt *encrypt.Encrypter
}

var errNoSalesBlinkEncrypter = errors.New("credential encrypter not configured (set CREDENTIALS_ENCRYPTION_KEY)")

// NewSalesBlinkRepository seals the API key with the mailbox credential key.
func NewSalesBlinkRepository(db *pgxpool.Pool, enc *encrypt.Encrypter) SalesBlinkRepository {
	return &salesBlinkRepository{db: db, encrypt: enc}
}

func (r *salesBlinkRepository) Get(ctx context.Context) (*models.SalesBlinkLink, error) {
	query := `
		SELECT base_url, api_key, auto_enroll, warmup_per_day, max_daily_frequency,
		       ramp_up_frequency, auto_ramp_up, connected_by, connected_at,
		       last_synced_at, last_error
		FROM salesblink_link WHERE id = true`
	var l models.SalesBlinkLink
	var sealed string
	err := r.db.QueryRow(ctx, query).Scan(
		&l.BaseURL, &sealed, &l.AutoEnroll, &l.WarmupPerDay, &l.MaxDailyFrequency,
		&l.RampUpFrequency, &l.AutoRampUp, &l.ConnectedBy, &l.ConnectedAt,
		&l.LastSyncedAt, &l.LastError,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		db.CaptureError(err, query, nil, "queryrow")
		return nil, err
	}
	if r.encrypt == nil {
		return nil, errNoSalesBlinkEncrypter
	}
	plain, err := r.encrypt.Decrypt(sealed)
	if err != nil {
		return nil, err
	}
	l.APIKey = plain
	return &l, nil
}

func (r *salesBlinkRepository) Put(ctx context.Context, link *models.SalesBlinkLink) error {
	if r.encrypt == nil {
		return errNoSalesBlinkEncrypter
	}
	sealed, err := r.encrypt.Encrypt(link.APIKey)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO salesblink_link (id, base_url, api_key, auto_enroll, warmup_per_day,
		            max_daily_frequency, ramp_up_frequency, auto_ramp_up, connected_by,
		            connected_at, last_error)
		VALUES (true, $1, $2, $3, $4, $5, $6, $7, $8, NOW(), '')
		ON CONFLICT (id) DO UPDATE SET
		  base_url = EXCLUDED.base_url, api_key = EXCLUDED.api_key,
		  auto_enroll = EXCLUDED.auto_enroll, warmup_per_day = EXCLUDED.warmup_per_day,
		  max_daily_frequency = EXCLUDED.max_daily_frequency,
		  ramp_up_frequency = EXCLUDED.ramp_up_frequency,
		  auto_ramp_up = EXCLUDED.auto_ramp_up, connected_by = EXCLUDED.connected_by,
		  connected_at = NOW(), last_error = ''
		RETURNING connected_at`
	if err := r.db.QueryRow(ctx, query, link.BaseURL, sealed, link.AutoEnroll,
		link.WarmupPerDay, link.MaxDailyFrequency, link.RampUpFrequency,
		link.AutoRampUp, link.ConnectedBy).Scan(&link.ConnectedAt); err != nil {
		db.CaptureError(err, query, nil, "queryrow")
		return err
	}
	return nil
}

func (r *salesBlinkRepository) Delete(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM salesblink_link WHERE id = true`); err != nil {
		db.CaptureError(err, "delete salesblink_link", nil, "exec")
		return err
	}
	return nil
}

func (r *salesBlinkRepository) SetSyncResult(ctx context.Context, at time.Time, lastError string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE salesblink_link
		    SET last_synced_at = CASE WHEN $2 = '' THEN $1 ELSE last_synced_at END,
		        last_error = $2
		  WHERE id = true`, at, lastError)
	return err
}

func (r *salesBlinkRepository) Enroll(ctx context.Context, accountID uuid.UUID, senderID string) (*models.SalesBlinkMailbox, error) {
	query := `
		INSERT INTO salesblink_mailboxes (email_account_id, sender_id)
		VALUES ($1, $2)
		ON CONFLICT (email_account_id) DO UPDATE SET sender_id = EXCLUDED.sender_id, last_error = ''
		RETURNING email_account_id, sender_id, enrolled_at, health_score, last_stats_at, last_error`
	var m models.SalesBlinkMailbox
	if err := r.db.QueryRow(ctx, query, accountID, senderID).Scan(
		&m.EmailAccountID, &m.SenderID, &m.EnrolledAt, &m.HealthScore, &m.LastStatsAt, &m.LastError,
	); err != nil {
		db.CaptureError(err, query, nil, "queryrow")
		return nil, err
	}
	return &m, nil
}

func (r *salesBlinkRepository) Unenroll(ctx context.Context, accountID uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM salesblink_mailboxes WHERE email_account_id = $1`, accountID); err != nil {
		db.CaptureError(err, "delete salesblink_mailboxes", []any{accountID}, "exec")
		return err
	}
	return nil
}

func (r *salesBlinkRepository) UnenrollAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `DELETE FROM salesblink_mailboxes`)
	return err
}

func (r *salesBlinkRepository) GetByAccount(ctx context.Context, accountID uuid.UUID) (*models.SalesBlinkMailbox, error) {
	query := `SELECT email_account_id, sender_id, enrolled_at, health_score, last_stats_at, last_error
	            FROM salesblink_mailboxes WHERE email_account_id = $1`
	var m models.SalesBlinkMailbox
	if err := r.db.QueryRow(ctx, query, accountID).Scan(
		&m.EmailAccountID, &m.SenderID, &m.EnrolledAt, &m.HealthScore, &m.LastStatsAt, &m.LastError,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		db.CaptureError(err, query, []any{accountID}, "queryrow")
		return nil, err
	}
	return &m, nil
}

func (r *salesBlinkRepository) List(ctx context.Context) ([]models.SalesBlinkMailbox, error) {
	query := `SELECT email_account_id, sender_id, enrolled_at, health_score, last_stats_at, last_error
	            FROM salesblink_mailboxes ORDER BY enrolled_at`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		db.CaptureError(err, query, nil, "query")
		return nil, err
	}
	defer rows.Close()
	out := []models.SalesBlinkMailbox{}
	for rows.Next() {
		var m models.SalesBlinkMailbox
		if err := rows.Scan(&m.EmailAccountID, &m.SenderID, &m.EnrolledAt, &m.HealthScore, &m.LastStatsAt, &m.LastError); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *salesBlinkRepository) IsEnrolled(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var ok bool
	if err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM salesblink_mailboxes WHERE email_account_id = $1)`, accountID).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *salesBlinkRepository) SetHealth(ctx context.Context, accountID uuid.UUID, score *int, lastError string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE salesblink_mailboxes
		    SET health_score = $2, last_stats_at = NOW(), last_error = $3
		  WHERE email_account_id = $1`, accountID, score, lastError)
	return err
}
