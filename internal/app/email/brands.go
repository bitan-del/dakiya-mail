package email

import (
	"context"
	"net/url"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
)

func validateBrand(req *models.SaveBrandRequest) *errx.Error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 80 {
		return errx.NewWithIdentifier(errx.BadRequest, "invalid_brand_name", "name must be 1 to 80 characters")
	}
	if w := strings.TrimSpace(req.Website); w != "" {
		u, err := url.Parse(w)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errx.NewWithIdentifier(errx.BadRequest, "invalid_brand_website", "website must be an http(s) URL")
		}
	}
	if len(req.PhysicalAddress) > 300 || len(req.FooterText) > 500 {
		return errx.New(errx.BadRequest, "physical_address is at most 300 characters and footer_text at most 500")
	}
	return nil
}

func (s *emailService) ListBrands(ctx context.Context, orgID uuid.UUID) ([]models.Brand, *errx.Error) {
	return s.emailRepository.ListBrands(ctx, orgID)
}

func (s *emailService) GetBrand(ctx context.Context, orgID, id uuid.UUID) (*models.Brand, *errx.Error) {
	return s.emailRepository.GetBrand(ctx, orgID, id)
}

func (s *emailService) SaveBrand(ctx context.Context, orgID uuid.UUID, id *uuid.UUID, req *models.SaveBrandRequest) (*models.Brand, *errx.Error) {
	if xerr := validateBrand(req); xerr != nil {
		return nil, xerr
	}
	saved, xerr := s.emailRepository.SaveBrand(ctx, orgID, id, req)
	if xerr != nil {
		return nil, xerr
	}
	return s.emailRepository.GetBrand(ctx, orgID, saved)
}

func (s *emailService) DeleteBrand(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	return s.emailRepository.DeleteBrand(ctx, orgID, id)
}

// SetBrandMailboxes replaces the brand's mailboxes; a mailbox moves off any other brand.
func (s *emailService) SetBrandMailboxes(ctx context.Context, orgID, brandID uuid.UUID, rawIDs []string) (*models.Brand, *errx.Error) {
	if _, xerr := s.emailRepository.GetBrand(ctx, orgID, brandID); xerr != nil {
		return nil, xerr
	}
	if len(rawIDs) > 1000 {
		return nil, errx.New(errx.BadRequest, "at most 1000 mailboxes per brand")
	}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, errx.New(errx.BadRequest, "email_account_ids must be mailbox ids")
		}
		ids = append(ids, id)
	}
	if xerr := s.emailRepository.SetBrandMailboxes(ctx, orgID, brandID, ids); xerr != nil {
		return nil, xerr
	}
	return s.emailRepository.GetBrand(ctx, orgID, brandID)
}
