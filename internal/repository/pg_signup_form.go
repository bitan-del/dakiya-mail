package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSlugTaken is returned when another form already uses the slug.
var ErrSlugTaken = errors.New("slug taken")

type SignupFormRepository interface {
	List(ctx context.Context, orgID uuid.UUID) ([]models.SignupForm, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*models.SignupForm, error)
	GetBySlug(ctx context.Context, slug string) (*models.SignupForm, error)
	Save(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, createdBy *uuid.UUID, f *models.SignupForm) (uuid.UUID, error)
	Delete(ctx context.Context, orgID, id uuid.UUID) (bool, error)
	CountSubmission(ctx context.Context, id uuid.UUID, confirmed bool) error
	CreatePending(ctx context.Context, formID uuid.UUID, tokenHash, email string, data *models.SignupSubmission) error
	RecentPending(ctx context.Context, formID uuid.UUID, email string, since time.Time) (bool, error)
	ClaimPending(ctx context.Context, formID uuid.UUID, tokenHash string, notBefore time.Time) (*models.SignupSubmission, error)
}

type signupFormRepository struct{ db *pgxpool.Pool }

func NewSignupFormRepository(pool *pgxpool.Pool) SignupFormRepository {
	return &signupFormRepository{db: pool}
}

const signupFormColumns = `f.id, f.organization_id, f.created_by, f.name, f.slug, f.title, f.description, f.fields, f.category_ids,
	f.double_opt_in, f.email_account_id, f.success_message, f.redirect_url, f.enabled, f.submissions, f.confirmations, f.created_at, f.updated_at,
	COALESCE(f.created_by, o.owner_user_id)`

func scanSignupForm(row pgx.Row) (*models.SignupForm, error) {
	var f models.SignupForm
	if err := row.Scan(&f.ID, &f.OrganizationID, &f.CreatedBy, &f.Name, &f.Slug, &f.Title, &f.Description, &f.Fields, &f.CategoryIDs,
		&f.DoubleOptIn, &f.EmailAccountID, &f.SuccessMessage, &f.RedirectURL, &f.Enabled, &f.Submissions, &f.Confirmations, &f.CreatedAt, &f.UpdatedAt,
		&f.OwnerUserID); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *signupFormRepository) one(ctx context.Context, where string, args ...any) (*models.SignupForm, error) {
	f, err := scanSignupForm(r.db.QueryRow(ctx, `SELECT `+signupFormColumns+` FROM signup_forms f JOIN organizations o ON o.id = f.organization_id WHERE `+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return f, err
}

func (r *signupFormRepository) List(ctx context.Context, orgID uuid.UUID) ([]models.SignupForm, error) {
	rows, err := r.db.Query(ctx, `SELECT `+signupFormColumns+` FROM signup_forms f JOIN organizations o ON o.id = f.organization_id WHERE f.organization_id = $1 ORDER BY f.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SignupForm{}
	for rows.Next() {
		f, err := scanSignupForm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

func (r *signupFormRepository) Get(ctx context.Context, orgID, id uuid.UUID) (*models.SignupForm, error) {
	return r.one(ctx, `f.organization_id = $1 AND f.id = $2`, orgID, id)
}

func (r *signupFormRepository) GetBySlug(ctx context.Context, slug string) (*models.SignupForm, error) {
	return r.one(ctx, `f.slug = $1`, slug)
}

func (r *signupFormRepository) Save(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, createdBy *uuid.UUID, f *models.SignupForm) (uuid.UUID, error) {
	args := []any{orgID, f.Name, f.Slug, f.Title, f.Description, f.Fields, f.CategoryIDs, f.DoubleOptIn, f.EmailAccountID, f.SuccessMessage, f.RedirectURL, f.Enabled}
	var out uuid.UUID
	var err error
	if id == nil {
		err = r.db.QueryRow(ctx, `
			INSERT INTO signup_forms (organization_id, name, slug, title, description, fields, category_ids, double_opt_in, email_account_id, success_message, redirect_url, enabled, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`, append(args, createdBy)...).Scan(&out)
	} else {
		err = r.db.QueryRow(ctx, `
			UPDATE signup_forms SET name = $2, slug = $3, title = $4, description = $5, fields = $6, category_ids = $7, double_opt_in = $8,
			       email_account_id = $9, success_message = $10, redirect_url = $11, enabled = $12, updated_at = NOW()
			WHERE organization_id = $1 AND id = $13 RETURNING id`, append(args, *id)...).Scan(&out)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return uuid.Nil, ErrSlugTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return out, err
}

func (r *signupFormRepository) Delete(ctx context.Context, orgID, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM signup_forms WHERE organization_id = $1 AND id = $2`, orgID, id)
	return tag.RowsAffected() > 0, err
}

func (r *signupFormRepository) CountSubmission(ctx context.Context, id uuid.UUID, confirmed bool) error {
	col := "submissions"
	if confirmed {
		col = "confirmations"
	}
	_, err := r.db.Exec(ctx, `UPDATE signup_forms SET `+col+` = `+col+` + 1 WHERE id = $1`, id)
	return err
}

func (r *signupFormRepository) CreatePending(ctx context.Context, formID uuid.UUID, tokenHash, email string, data *models.SignupSubmission) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO signup_form_pending (token_hash, form_id, email, data) VALUES ($1, $2, $3, $4)`, tokenHash, formID, email, raw)
	return err
}

func (r *signupFormRepository) RecentPending(ctx context.Context, formID uuid.UUID, email string, since time.Time) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signup_form_pending WHERE form_id = $1 AND email = $2 AND created_at >= $3)`, formID, email, since).Scan(&ok)
	return ok, err
}

// ClaimPending marks a pending submission confirmed and returns it, once.
func (r *signupFormRepository) ClaimPending(ctx context.Context, formID uuid.UUID, tokenHash string, notBefore time.Time) (*models.SignupSubmission, error) {
	var raw []byte
	err := r.db.QueryRow(ctx, `
		UPDATE signup_form_pending SET confirmed_at = NOW()
		WHERE token_hash = $1 AND form_id = $2 AND confirmed_at IS NULL AND created_at >= $3
		RETURNING data`, tokenHash, formID, notBefore).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sub models.SignupSubmission
	if err := json.Unmarshal(raw, &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}
