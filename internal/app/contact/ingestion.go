package contact

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
)

// IngestionRulesSource supplies the workspace's contact ingestion rules.
type IngestionRulesSource interface {
	GetOutreachSettings(ctx context.Context, organizationID uuid.UUID) (*models.AdvancedOutreachSettings, error)
}

// IngestionAware is the optional capability the caller uses to attach it.
type IngestionAware interface {
	WireIngestionRules(src IngestionRulesSource)
}

func (s *contactService) WireIngestionRules(src IngestionRulesSource) { s.ingestion = src }

func (s *contactService) ingestionRules(ctx context.Context, orgID uuid.UUID) models.ContactIngestionSettings {
	if s.ingestion == nil {
		return models.ContactIngestionSettings{}
	}
	settings, err := s.ingestion.GetOutreachSettings(ctx, orgID)
	if err != nil || settings == nil {
		return models.ContactIngestionSettings{}
	}
	return settings.ContactIngestion
}

func missingRequiredField(c *models.AddContact, fields []string) string {
	for _, f := range fields {
		var v string
		switch f {
		case "first_name":
			v = c.FirstName
		case "last_name":
			v = c.LastName
		case "company":
			v = c.Company
		case "phone":
			v = c.Phone
		default:
			v = c.CustomFields[strings.TrimPrefix(f, "custom:")]
		}
		if strings.TrimSpace(v) == "" {
			return f
		}
	}
	return ""
}

// screenContacts returns, per contact, why the ingestion rules reject it ("" = accepted).
func (s *contactService) screenContacts(ctx context.Context, orgID uuid.UUID, contacts []*models.AddContact) ([]string, *errx.Error) {
	reasons := make([]string, len(contacts))
	rules := s.ingestionRules(ctx, orgID)
	if rules.DuplicateWindowDays <= 0 && len(rules.RequiredFields) == 0 {
		return reasons, nil
	}
	for i, c := range contacts {
		if f := missingRequiredField(c, rules.RequiredFields); f != "" {
			reasons[i] = fmt.Sprintf("missing required field %s", strings.TrimPrefix(f, "custom:"))
		}
	}
	if rules.DuplicateWindowDays > 0 {
		emails := make([]string, 0, len(contacts))
		for i, c := range contacts {
			if reasons[i] == "" {
				emails = append(emails, strings.ToLower(strings.TrimSpace(c.Email)))
			}
		}
		since := time.Now().Add(-time.Duration(rules.DuplicateWindowDays) * 24 * time.Hour)
		recent, xerr := s.contactRepository.RecentContactEmails(ctx, orgID, emails, since)
		if xerr != nil {
			return nil, xerr
		}
		for i, c := range contacts {
			if reasons[i] == "" && recent[strings.ToLower(strings.TrimSpace(c.Email))] {
				reasons[i] = fmt.Sprintf("duplicate: already added in the last %d days", rules.DuplicateWindowDays)
			}
		}
	}
	return reasons, nil
}
