package contact

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/google/uuid"
)

type fakeIngestionRules struct {
	rules models.ContactIngestionSettings
}

func (f fakeIngestionRules) GetOutreachSettings(context.Context, uuid.UUID) (*models.AdvancedOutreachSettings, error) {
	return &models.AdvancedOutreachSettings{ContactIngestion: f.rules}, nil
}

type fakeRecentRepo struct {
	repository.ContactRepository
	recent map[string]bool
	since  time.Time
}

func (f *fakeRecentRepo) RecentContactEmails(_ context.Context, _ uuid.UUID, emails []string, since time.Time) (map[string]bool, *errx.Error) {
	f.since = since
	out := map[string]bool{}
	for _, e := range emails {
		if f.recent[e] {
			out[e] = true
		}
	}
	return out, nil
}

func TestScreenContacts(t *testing.T) {
	repo := &fakeRecentRepo{recent: map[string]bool{"seen@example.com": true}}
	s := &contactService{contactRepository: repo}
	s.WireIngestionRules(fakeIngestionRules{models.ContactIngestionSettings{
		DuplicateWindowDays: 30,
		RequiredFields:      []string{"first_name", "custom:source_id"},
	}})

	contacts := []*models.AddContact{
		{Email: "new@example.com", FirstName: "Ana", CustomFields: map[string]string{"source_id": "s1"}},
		{Email: "Seen@Example.com", FirstName: "Bo", CustomFields: map[string]string{"source_id": "s1"}},
		{Email: "nofirst@example.com", CustomFields: map[string]string{"source_id": "s1"}},
		{Email: "nosid@example.com", FirstName: "Cy"},
	}
	reasons, xerr := s.screenContacts(context.Background(), uuid.New(), contacts)
	if xerr != nil {
		t.Fatal(xerr)
	}
	if reasons[0] != "" {
		t.Errorf("valid contact rejected: %q", reasons[0])
	}
	if !strings.Contains(reasons[1], "duplicate") {
		t.Errorf("recent duplicate not rejected: %q", reasons[1])
	}
	if !strings.Contains(reasons[2], "first_name") || !strings.Contains(reasons[3], "source_id") {
		t.Errorf("missing fields not rejected: %q / %q", reasons[2], reasons[3])
	}
	if d := time.Since(repo.since); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Errorf("window start = %s ago, want about 30 days", d)
	}

	// No rules means nothing is screened and the repository is not queried.
	plain := &contactService{contactRepository: &fakeRecentRepo{}}
	if r, _ := plain.screenContacts(context.Background(), uuid.New(), contacts); r[1] != "" {
		t.Errorf("screened without rules: %q", r[1])
	}
}

func TestNormalizeIngestionSettings(t *testing.T) {
	s := &models.AdvancedOutreachSettings{ContactIngestion: models.ContactIngestionSettings{
		DuplicateWindowDays: 9999,
		RequiredFields:      []string{"first_name", "first_name", "bogus", "custom:", "custom:sid"},
	}}
	s.Normalize()
	if s.ContactIngestion.DuplicateWindowDays != models.MaxIngestionWindowDays {
		t.Errorf("window = %d", s.ContactIngestion.DuplicateWindowDays)
	}
	if got := strings.Join(s.ContactIngestion.RequiredFields, ","); got != "first_name,custom:sid" {
		t.Errorf("required fields = %s", got)
	}
}
