package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

type fakeCampaigns struct {
	repository.CampaignRepository
	campaign *models.Campaign
	err      error
}

func (f fakeCampaigns) GetByID(context.Context, uuid.UUID) (*models.Campaign, error) {
	return f.campaign, f.err
}

type fakeEmails struct {
	repository.EmailRepository
	explicit, tagged, all []uuid.UUID
}

func (f fakeEmails) GetByCampaignSenders(context.Context, repository.AccountScope, uuid.UUID) ([]repository.CampaignSenderAccount, *errx.Error) {
	out := []repository.CampaignSenderAccount{}
	for _, id := range f.explicit {
		out = append(out, repository.CampaignSenderAccount{Account: models.Email{ID: id}})
	}
	return out, nil
}

func (f fakeEmails) GetByTags(context.Context, repository.AccountScope, []string) ([]models.Email, *errx.Error) {
	return emails(f.tagged), nil
}

func (f fakeEmails) GetAllActiveInScope(context.Context, repository.AccountScope) ([]models.Email, *errx.Error) {
	return emails(f.all), nil
}

func emails(ids []uuid.UUID) []models.Email {
	out := []models.Email{}
	for _, id := range ids {
		out = append(out, models.Email{ID: id})
	}
	return out
}

type fakeAudience struct{ a repository.CampaignAudience }

func (f fakeAudience) GetCampaignAudience(context.Context, uuid.UUID, uuid.UUID) (repository.CampaignAudience, error) {
	return f.a, nil
}

type fakeRepo struct {
	asked  []uuid.UUID
	reads  int
	failOn string
}

func (f *fakeRepo) Mailboxes(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]repository.AdvisorMailbox, error) {
	f.reads++
	f.asked = ids
	if f.failOn == "mailboxes" {
		return nil, errors.New("boom")
	}
	out := []repository.AdvisorMailbox{}
	for range ids {
		out = append(out, healthy("a@x.io"))
	}
	return out, nil
}

func (f *fakeRepo) ListHistory(context.Context, uuid.UUID, uuid.UUID) (repository.CampaignListHistory, error) {
	f.reads++
	return repository.CampaignListHistory{}, nil
}

func newTestService(c *models.Campaign, e fakeEmails, repo *fakeRepo) Service {
	return NewService(fakeCampaigns{campaign: c}, e, fakeAudience{cleanAudience(1000)}, repo)
}

func campaignIn(org uuid.UUID, tags ...string) *models.Campaign {
	return &models.Campaign{ID: uuid.New(), OrganizationID: &org, UnsubscribeHeader: true, DailyLimit: 60, EmailTags: tags}
}

func TestRunTreatsAnotherOrganizationsCampaignAsMissing(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(campaignIn(uuid.New()), fakeEmails{all: []uuid.UUID{uuid.New()}}, repo)

	_, xerr := svc.Run(context.Background(), uuid.New(), uuid.New())
	if xerr == nil || xerr.Code != errx.NotFound {
		t.Fatalf("got %v, want a 404", xerr)
	}
	if repo.reads != 0 {
		t.Errorf("read %d things about a campaign the caller does not own", repo.reads)
	}
}

func TestRunWithNoCampaignIsNotFound(t *testing.T) {
	svc := newTestService(nil, fakeEmails{}, &fakeRepo{})
	if _, xerr := svc.Run(context.Background(), uuid.New(), uuid.New()); xerr == nil || xerr.Code != errx.NotFound {
		t.Fatalf("got %v, want a 404", xerr)
	}
}

func TestRunBuildsTheReportFromTheCampaign(t *testing.T) {
	org := uuid.New()
	c := campaignIn(org)
	c.UnsubscribeHeader = false
	svc := newTestService(c, fakeEmails{all: []uuid.UUID{uuid.New(), uuid.New()}}, &fakeRepo{})

	r, xerr := svc.Run(context.Background(), org, c.ID)
	if xerr != nil {
		t.Fatalf("run: %v", xerr)
	}
	if r.Verdict != StatusFail || r.CampaignID != c.ID {
		t.Errorf("verdict = %s for campaign %s, want fail for %s", r.Verdict, r.CampaignID, c.ID)
	}
	for _, ch := range r.Checks {
		if ch.Key == "unsubscribe_header" && ch.Status != StatusFail {
			t.Errorf("unsubscribe off should fail, got %s", ch.Status)
		}
	}
}

func TestRunSurfacesALoadFailureInsteadOfAGuess(t *testing.T) {
	org := uuid.New()
	c := campaignIn(org)
	svc := newTestService(c, fakeEmails{all: []uuid.UUID{uuid.New()}}, &fakeRepo{failOn: "mailboxes"})
	if _, xerr := svc.Run(context.Background(), org, c.ID); xerr == nil || xerr.Code != errx.Internal {
		t.Fatalf("got %v, want a 500 rather than a report built on missing data", xerr)
	}
}

func TestSenderPoolMirrorsTheScheduler(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	cases := []struct {
		name   string
		emails fakeEmails
		tags   []string
		want   []uuid.UUID
	}{
		{"explicit and tagged are one pool, deduplicated", fakeEmails{explicit: []uuid.UUID{a, b}, tagged: []uuid.UUID{b, c}}, []string{"t"}, []uuid.UUID{a, b, c}},
		{"explicit only", fakeEmails{explicit: []uuid.UUID{a}, all: []uuid.UUID{b, c}}, nil, []uuid.UUID{a}},
		{"tags only", fakeEmails{tagged: []uuid.UUID{c}, all: []uuid.UUID{a, b}}, []string{"t"}, []uuid.UUID{c}},
		{"neither means every active mailbox", fakeEmails{all: []uuid.UUID{a, b}}, nil, []uuid.UUID{a, b}},
	}
	for _, tc := range cases {
		org := uuid.New()
		repo := &fakeRepo{}
		camp := campaignIn(org, tc.tags...)
		if _, xerr := newTestService(camp, tc.emails, repo).Run(context.Background(), org, camp.ID); xerr != nil {
			t.Fatalf("%s: %v", tc.name, xerr)
		}
		if len(repo.asked) != len(tc.want) {
			t.Errorf("%s: asked for %v, want %v", tc.name, repo.asked, tc.want)
			continue
		}
		for i := range tc.want {
			if repo.asked[i] != tc.want[i] {
				t.Errorf("%s: asked for %v, want %v", tc.name, repo.asked, tc.want)
				break
			}
		}
	}
}
