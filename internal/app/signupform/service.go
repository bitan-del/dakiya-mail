// Package signupform runs hosted signup forms: the dashboard configures them,
// visitors submit them at /f/<slug>, and submissions become contacts, either at
// once or after the visitor confirms by email (double opt-in).
package signupform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/app/contact"
	"github.com/bitan-del/dakiya-mail/internal/app/emailsend"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	confirmWindow  = 7 * 24 * time.Hour
	resendCooldown = 10 * time.Minute
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

type Service interface {
	List(ctx context.Context, orgID uuid.UUID) ([]models.SignupForm, *errx.Error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*models.SignupForm, *errx.Error)
	Save(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, userID uuid.UUID, req *models.SaveSignupFormRequest) (*models.SignupForm, *errx.Error)
	Delete(ctx context.Context, orgID, id uuid.UUID) *errx.Error
	// Public returns an enabled form by slug, or nil.
	Public(ctx context.Context, slug string) (*models.SignupForm, *errx.Error)
	// Submit handles a visitor's post. needsConfirm reports a double opt-in email went out.
	Submit(ctx context.Context, form *models.SignupForm, sub *models.SignupSubmission, baseURL string) (needsConfirm bool, xerr *errx.Error)
	Confirm(ctx context.Context, form *models.SignupForm, token string) *errx.Error
}

type service struct {
	repo     repository.SignupFormRepository
	emails   repository.EmailRepository
	contacts contact.ContactService
	sender   emailsend.EmailSendService
}

func NewService(repo repository.SignupFormRepository, emails repository.EmailRepository, contacts contact.ContactService, sender emailsend.EmailSendService) Service {
	return &service{repo: repo, emails: emails, contacts: contacts, sender: sender}
}

func internal(err error) *errx.Error {
	log.Error().Err(err).Msg("signup form")
	return errx.InternalError()
}

func (s *service) List(ctx context.Context, orgID uuid.UUID) ([]models.SignupForm, *errx.Error) {
	list, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, internal(err)
	}
	return list, nil
}

func (s *service) Get(ctx context.Context, orgID, id uuid.UUID) (*models.SignupForm, *errx.Error) {
	f, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return nil, internal(err)
	}
	if f == nil {
		return nil, errx.New(errx.NotFound, "form not found")
	}
	return f, nil
}

func (s *service) build(ctx context.Context, orgID uuid.UUID, req *models.SaveSignupFormRequest) (*models.SignupForm, *errx.Error) {
	bad := func(id, msg string) *errx.Error { return errx.NewWithIdentifier(errx.BadRequest, id, msg) }
	f := &models.SignupForm{
		Name:           strings.TrimSpace(req.Name),
		Slug:           strings.ToLower(strings.TrimSpace(req.Slug)),
		Title:          strings.TrimSpace(req.Title),
		Description:    strings.TrimSpace(req.Description),
		DoubleOptIn:    req.DoubleOptIn,
		SuccessMessage: strings.TrimSpace(req.SuccessMessage),
		RedirectURL:    strings.TrimSpace(req.RedirectURL),
		Enabled:        req.Enabled == nil || *req.Enabled,
		Fields:         []string{},
		CategoryIDs:    []uuid.UUID{},
	}
	if f.Name == "" || len([]rune(f.Name)) > 80 {
		return nil, bad("invalid_form_name", "name must be 1 to 80 characters")
	}
	if !slugPattern.MatchString(f.Slug) {
		return nil, bad("invalid_form_slug", "slug must be 3 to 63 lowercase letters, digits or dashes")
	}
	if len(f.Title) > 120 || len(f.Description) > 1000 || len(f.SuccessMessage) > 500 {
		return nil, errx.New(errx.BadRequest, "title is at most 120 characters, description 1000 and success_message 500")
	}
	if f.RedirectURL != "" {
		u, err := url.Parse(f.RedirectURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, bad("invalid_form_redirect", "redirect_url must be an https URL")
		}
	}
	for _, fld := range req.Fields {
		if !slices.Contains(models.SignupFormFields, fld) {
			return nil, bad("invalid_form_field", "fields may be first_name, last_name, company and phone")
		}
		if !slices.Contains(f.Fields, fld) {
			f.Fields = append(f.Fields, fld)
		}
	}
	for _, raw := range req.CategoryIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, errx.New(errx.BadRequest, "category_ids must be category ids")
		}
		f.CategoryIDs = append(f.CategoryIDs, id)
	}
	if raw := strings.TrimSpace(req.EmailAccountID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, bad("invalid_form_mailbox", "email_account_id is not a valid id")
		}
		if acc, xerr := s.emails.Get(ctx, orgID.String(), id.String()); xerr != nil || acc == nil {
			return nil, bad("invalid_form_mailbox", "email_account_id is not a mailbox in this workspace")
		}
		f.EmailAccountID = &id
	}
	if f.DoubleOptIn && f.EmailAccountID == nil {
		return nil, bad("invalid_form_mailbox", "double opt-in needs a mailbox to send the confirmation from")
	}
	return f, nil
}

func (s *service) Save(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, userID uuid.UUID, req *models.SaveSignupFormRequest) (*models.SignupForm, *errx.Error) {
	f, xerr := s.build(ctx, orgID, req)
	if xerr != nil {
		return nil, xerr
	}
	saved, err := s.repo.Save(ctx, orgID, id, &userID, f)
	if err == repository.ErrSlugTaken {
		return nil, errx.NewWithIdentifier(errx.Conflict, "form_slug_taken", "that form address is already in use")
	}
	if err != nil {
		return nil, internal(err)
	}
	if saved == uuid.Nil {
		return nil, errx.New(errx.NotFound, "form not found")
	}
	return s.Get(ctx, orgID, saved)
}

func (s *service) Delete(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	ok, err := s.repo.Delete(ctx, orgID, id)
	if err != nil {
		return internal(err)
	}
	if !ok {
		return errx.New(errx.NotFound, "form not found")
	}
	return nil
}

func (s *service) Public(ctx context.Context, slug string) (*models.SignupForm, *errx.Error) {
	f, err := s.repo.GetBySlug(ctx, strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		return nil, internal(err)
	}
	if f == nil || !f.Enabled {
		return nil, nil
	}
	return f, nil
}

func (s *service) addContact(ctx context.Context, f *models.SignupForm, sub *models.SignupSubmission) *errx.Error {
	subscribed := true
	cats := make([]string, 0, len(f.CategoryIDs))
	for _, c := range f.CategoryIDs {
		cats = append(cats, c.String())
	}
	in := models.AddContact{
		Email:        strings.ToLower(strings.TrimSpace(sub.Email)),
		Categories:   cats,
		Campaigns:    []string{},
		CustomFields: map[string]string{"source": "form:" + f.Slug},
		Subscribed:   &subscribed,
		Source:       models.ContactSourceForm,
		SourceDetail: f.Name,
	}
	if slices.Contains(f.Fields, "first_name") {
		in.FirstName = strings.TrimSpace(sub.FirstName)
	}
	if slices.Contains(f.Fields, "last_name") {
		in.LastName = strings.TrimSpace(sub.LastName)
	}
	if slices.Contains(f.Fields, "company") {
		in.Company = strings.TrimSpace(sub.Company)
	}
	if slices.Contains(f.Fields, "phone") {
		in.Phone = strings.TrimSpace(sub.Phone)
	}
	_, xerr := s.contacts.Add(ctx, f.OwnerUserID.String(), f.OrganizationID, []models.AddContact{in})
	// An address the workspace's intake rules refuse still sees a normal
	// confirmation, so a form never reveals who is already on the list.
	if xerr != nil && xerr.Identifier == "contact_ingestion_rejected" {
		return nil
	}
	return xerr
}

func (s *service) Submit(ctx context.Context, f *models.SignupForm, sub *models.SignupSubmission, baseURL string) (bool, *errx.Error) {
	if strings.TrimSpace(sub.Honeypot) != "" {
		return f.DoubleOptIn, nil
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(sub.Email))
	if err != nil || len(addr.Address) > 254 {
		return false, errx.NewWithIdentifier(errx.BadRequest, "invalid_email", "Enter a valid email address.")
	}
	sub.Email = strings.ToLower(addr.Address)
	if !f.DoubleOptIn {
		if xerr := s.addContact(ctx, f, sub); xerr != nil {
			return false, xerr
		}
		_ = s.repo.CountSubmission(ctx, f.ID, false)
		return false, nil
	}

	// One confirmation per address per few minutes, so a form cannot be used to flood an inbox.
	if recent, err := s.repo.RecentPending(ctx, f.ID, sub.Email, time.Now().Add(-resendCooldown)); err != nil {
		return false, internal(err)
	} else if recent {
		return true, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return false, internal(err)
	}
	token := hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	if err := s.repo.CreatePending(ctx, f.ID, hex.EncodeToString(sum[:]), sub.Email, sub); err != nil {
		return false, internal(err)
	}
	link := strings.TrimSuffix(baseURL, "/") + "/f/" + f.Slug + "/confirm?t=" + token
	title := f.Title
	if title == "" {
		title = f.Name
	}
	_, xerr := s.sender.SendEmail(ctx, f.OwnerUserID, f.OrganizationID, *f.EmailAccountID, &emailsend.SendEmailRequest{
		To:      []string{sub.Email},
		Subject: "Confirm your subscription: " + title,
		BodyPlain: fmt.Sprintf("Please confirm you want to receive email from %s:\n\n%s\n\nIf you did not sign up, ignore this message.",
			title, link),
		BodyHTML: fmt.Sprintf(`<p>Please confirm you want to receive email from %s.</p><p><a href="%s">Confirm my subscription</a></p><p style="color:#888">If you did not sign up, ignore this message.</p>`,
			html.EscapeString(title), html.EscapeString(link)),
		SendMode:    "instant",
		NoUndoDelay: true,
	})
	if xerr != nil {
		return false, xerr
	}
	_ = s.repo.CountSubmission(ctx, f.ID, false)
	return true, nil
}

func (s *service) Confirm(ctx context.Context, f *models.SignupForm, token string) *errx.Error {
	token = strings.TrimSpace(token)
	if len(token) != 64 {
		return errx.New(errx.BadRequest, "This confirmation link is invalid.")
	}
	sum := sha256.Sum256([]byte(token))
	sub, err := s.repo.ClaimPending(ctx, f.ID, hex.EncodeToString(sum[:]), time.Now().Add(-confirmWindow))
	if err != nil {
		return internal(err)
	}
	if sub == nil {
		return errx.New(errx.BadRequest, "This confirmation link has expired or was already used.")
	}
	if xerr := s.addContact(ctx, f, sub); xerr != nil {
		return xerr
	}
	_ = s.repo.CountSubmission(ctx, f.ID, true)
	return nil
}
