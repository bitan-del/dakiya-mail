package postmaster

import (
	"context"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

func fp(v float64) *float64 { return &v }
func sp(v string) *string   { return &v }

func TestSummarize(t *testing.T) {
	days := []models.PostmasterDay{
		// Out of order on purpose: the latest is decided by date, not by position.
		{Date: "2026-01-03", DomainReputation: sp("medium"), SpamRate: fp(0.0031), SPFRatio: fp(1), DKIMRatio: fp(0.9), DMARCRatio: fp(1)},
		{Date: "2026-01-01", DomainReputation: sp("high"), SpamRate: fp(0.0004), SPFRatio: fp(1), DKIMRatio: fp(1), DMARCRatio: fp(0.8)},
		// A reporting day without reputation: reputation stays on the most recent day that had one.
		{Date: "2026-01-04", SpamRate: fp(0.001), SPFRatio: fp(0.5)},
		// A day Gmail named but filled nothing: not a day with data.
		{Date: "2026-01-05"},
		{Date: "2026-01-02", SpamRate: fp(0)},
	}
	got := Summarize(days)

	if got.DaysWithData != 4 {
		t.Errorf("days with data = %d, want 4 (the empty day does not count)", got.DaysWithData)
	}
	if got.LatestDate == nil || *got.LatestDate != "2026-01-04" {
		t.Errorf("latest date = %v, want 2026-01-04", got.LatestDate)
	}
	if got.LatestReputation == nil || *got.LatestReputation != "medium" {
		t.Errorf("latest reputation = %v, want medium", got.LatestReputation)
	}
	if got.SpamRateMax == nil || *got.SpamRateMax != 0.0031 {
		t.Errorf("spam max = %v", got.SpamRateMax)
	}
	if want := (0.0031 + 0.0004 + 0.001 + 0) / 4; got.SpamRateAvg == nil || abs(*got.SpamRateAvg-want) > 1e-12 {
		t.Errorf("spam avg = %v, want %v (a zero day counts, a missing one does not)", got.SpamRateAvg, want)
	}
	if got.DaysOverWarn != 2 || got.DaysOverCritical != 1 {
		t.Errorf("over warn/critical = %d/%d, want 2/1 (the line itself counts as reaching it)", got.DaysOverWarn, got.DaysOverCritical)
	}
	if got.SPFAvg == nil || abs(*got.SPFAvg-(1+1+0.5)/3) > 1e-12 {
		t.Errorf("spf avg = %v", got.SPFAvg)
	}
	if got.DKIMAvg == nil || abs(*got.DKIMAvg-0.95) > 1e-12 {
		t.Errorf("dkim avg = %v", got.DKIMAvg)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	got := Summarize(nil)
	if got.DaysWithData != 0 || got.LatestDate != nil || got.LatestReputation != nil ||
		got.SpamRateAvg != nil || got.SpamRateMax != nil || got.SPFAvg != nil || got.DKIMAvg != nil || got.DMARCAvg != nil {
		t.Errorf("no data must summarise to nothing, not zeros: %+v", got)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

type upsert struct {
	org    uuid.UUID
	domain string
	n      int
}

type fakeRepo struct {
	conn    *models.PostmasterConnection
	targets []models.PostmasterOrgDomain
	domains []models.PostmasterDomain
	stats   []models.PostmasterDay

	stored    *models.PostmasterCredentials
	deleted   bool
	upserts   []upsert
	syncedAt  *time.Time
	syncError string
	noSealer  bool
}

func (r *fakeRepo) GetConnection(context.Context) (*models.PostmasterConnection, error) {
	return r.conn, nil
}
func (r *fakeRepo) PutCredentials(_ context.Context, c models.PostmasterCredentials) error {
	if r.noSealer {
		return repository.ErrPostmasterNoEncrypter
	}
	r.stored = &c
	r.conn = &models.PostmasterConnection{Credentials: c}
	return nil
}
func (r *fakeRepo) DeleteCredentials(context.Context) error { r.deleted = true; return nil }
func (r *fakeRepo) SetSyncResult(_ context.Context, at time.Time, e string) error {
	r.syncedAt, r.syncError = &at, e
	return nil
}
func (r *fakeRepo) MailboxDomains(context.Context) ([]models.PostmasterOrgDomain, error) {
	return r.targets, nil
}
func (r *fakeRepo) OrgDomains(context.Context, uuid.UUID) ([]models.PostmasterDomain, error) {
	return r.domains, nil
}
func (r *fakeRepo) UpsertStats(_ context.Context, org uuid.UUID, domain string, s []models.PostmasterStat) error {
	r.upserts = append(r.upserts, upsert{org, domain, len(s)})
	return nil
}
func (r *fakeRepo) ListStats(context.Context, uuid.UUID, string, int) ([]models.PostmasterDay, error) {
	return r.stats, nil
}

var testCreds = models.PostmasterCredentials{ClientID: "cid", ClientSecret: "secret", RefreshToken: "rt"}

// serviceFor wires a service whose Google client talks to the fake.
func serviceFor(t *testing.T, repo *fakeRepo, f *fakeGoogle) *service {
	t.Helper()
	return &service{repo: repo, newClient: func(ctx context.Context, c models.PostmasterCredentials) *Client {
		return newClient(ctx, c, f.srv.URL, f.srv.URL+"/token", f.srv.Client())
	}}
}

func TestSyncOneCallPerDomainAndFansOutToWorkspaces(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	repo := &fakeRepo{
		conn: &models.PostmasterConnection{Credentials: testCreds},
		targets: []models.PostmasterOrgDomain{
			{OrganizationID: orgA, Domain: "shared.com"},
			{OrganizationID: orgB, Domain: "shared.com"},
			{OrganizationID: orgA, Domain: "unverified.com"},
		},
	}
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/domains/unverified.com/trafficStats" {
			http.Error(w, `{"error":{"status":"PERMISSION_DENIED"}}`, http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"trafficStats":[{"name":"domains/shared.com/trafficStats/20260101","domainReputation":"HIGH"}]}`)
	}

	if err := serviceFor(t, repo, f).Sync(context.Background(), 30); err != nil {
		t.Fatal(err)
	}

	sort.Slice(repo.upserts, func(i, j int) bool { return repo.upserts[i].org.String() < repo.upserts[j].org.String() })
	if len(repo.upserts) != 2 || repo.upserts[0].domain != "shared.com" || repo.upserts[1].domain != "shared.com" ||
		repo.upserts[0].n != 1 || repo.upserts[0].org == repo.upserts[1].org {
		t.Errorf("both workspaces sending from shared.com must get its stats: %+v", repo.upserts)
	}
	if f.v1Calls != 2 {
		t.Errorf("want one Google call per distinct domain (2), got %d", f.v1Calls)
	}
	if repo.syncedAt == nil || repo.syncError != "" {
		t.Errorf("an unverified domain is not a sync failure: at=%v err=%q", repo.syncedAt, repo.syncError)
	}
}

func TestSyncRecordsAccountFailureAndStops(t *testing.T) {
	repo := &fakeRepo{
		conn: &models.PostmasterConnection{Credentials: testCreds},
		targets: []models.PostmasterOrgDomain{
			{OrganizationID: uuid.New(), Domain: "a.com"},
			{OrganizationID: uuid.New(), Domain: "b.com"},
		},
	}
	f := newFakeGoogle(t)
	f.token = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
	}
	f.v1 = func(w http.ResponseWriter, r *http.Request) {}

	if err := serviceFor(t, repo, f).Sync(context.Background(), 30); err != nil {
		t.Fatalf("a Google-side failure is recorded, not returned: %v", err)
	}
	if repo.syncError == "" || repo.syncedAt == nil {
		t.Errorf("the refusal must be recorded for the dashboard: %q", repo.syncError)
	}
	// One domain tried (v1 then the v2 fallback, a token attempt each), the second never reached.
	if f.tokenHit > 2 || len(repo.upserts) != 0 {
		t.Errorf("an account refusal must stop the pass: tokenHits=%d upserts=%d", f.tokenHit, len(repo.upserts))
	}
}

func TestSyncWithoutCredentialsDoesNothing(t *testing.T) {
	repo := &fakeRepo{}
	f := newFakeGoogle(t)
	if err := serviceFor(t, repo, f).Sync(context.Background(), 30); err != nil {
		t.Fatal(err)
	}
	if f.tokenHit != 0 || repo.syncedAt != nil {
		t.Error("a disconnected instance must not call Google or record a sync")
	}
}

func TestReportPicksDomainWithData(t *testing.T) {
	latest := "2026-01-02"
	repo := &fakeRepo{
		conn:    &models.PostmasterConnection{},
		domains: []models.PostmasterDomain{{Domain: "empty.com", Mailboxes: 5}, {Domain: "data.com", Mailboxes: 1, LatestDate: &latest}},
		stats:   []models.PostmasterDay{{Date: "2026-01-02", SpamRate: fp(0.0005)}},
	}
	s := &service{repo: repo}

	rep, xerr := s.Report(context.Background(), uuid.New(), "", 30)
	if xerr != nil {
		t.Fatal(xerr)
	}
	if rep.Domain != "data.com" || !rep.Connected || len(rep.Data) != 1 || rep.Summary.DaysWithData != 1 {
		t.Errorf("default domain must be the first with data: %+v", rep)
	}
	if rep.Thresholds.SpamWarn != 0.001 || rep.Thresholds.SpamCritical != 0.003 {
		t.Errorf("thresholds: %+v", rep.Thresholds)
	}

	rep, xerr = s.Report(context.Background(), uuid.New(), " EMPTY.com ", 30)
	if xerr != nil || rep.Domain != "empty.com" {
		t.Errorf("an explicit domain is normalised and honoured: %+v %v", rep, xerr)
	}
}

func TestReportShapeWhenNothingIsConfigured(t *testing.T) {
	s := &service{repo: &fakeRepo{domains: []models.PostmasterDomain{}}}
	rep, xerr := s.Report(context.Background(), uuid.New(), "", 30)
	if xerr != nil {
		t.Fatal(xerr)
	}
	if rep.Connected || rep.Domain != "" || rep.Data == nil || len(rep.Data) != 0 || rep.Summary.DaysWithData != 0 {
		t.Errorf("not connected is an empty 200, never null data: %+v", rep)
	}
}

func TestReportRefusals(t *testing.T) {
	s := &service{repo: &fakeRepo{domains: []models.PostmasterDomain{{Domain: "mine.com"}}}}
	if _, xerr := s.Report(context.Background(), uuid.New(), "other.com", 30); xerr != ErrDomainNotFound {
		t.Errorf("a domain outside the workspace must be 404, got %v", xerr)
	}
	for _, days := range []int{0, -1, BackfillDays + 1} {
		if _, xerr := s.Report(context.Background(), uuid.New(), "", days); xerr != ErrInvalidDays {
			t.Errorf("days=%d must be refused, got %v", days, xerr)
		}
	}
}

func TestConnect(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")

	t.Run("stores valid credentials and runs the first sync", func(t *testing.T) {
		repo := &fakeRepo{targets: []models.PostmasterOrgDomain{{OrganizationID: uuid.New(), Domain: "mine.com"}}}
		f := newFakeGoogle(t)
		f.v1 = func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"trafficStats":[{"name":"domains/mine.com/trafficStats/20260101"}]}`)
		}
		xerr := serviceFor(t, repo, f).Connect(context.Background(), models.PostmasterCredentials{ClientID: " cid ", ClientSecret: "secret", RefreshToken: "rt"})
		if xerr != nil {
			t.Fatal(xerr)
		}
		if repo.stored == nil || repo.stored.ClientID != "cid" || len(repo.upserts) != 1 || repo.syncedAt == nil {
			t.Errorf("stored=%+v upserts=%v synced=%v", repo.stored, repo.upserts, repo.syncedAt)
		}
	})

	t.Run("rejects credentials Google refuses without storing them", func(t *testing.T) {
		repo := &fakeRepo{}
		f := newFakeGoogle(t)
		xerr := serviceFor(t, repo, f).Connect(context.Background(), models.PostmasterCredentials{ClientID: "cid", ClientSecret: "secret", RefreshToken: "wrong"})
		if xerr != ErrCredentials || repo.stored != nil {
			t.Errorf("xerr=%v stored=%+v", xerr, repo.stored)
		}
	})

	t.Run("rejects incomplete credentials before calling Google", func(t *testing.T) {
		f := newFakeGoogle(t)
		xerr := serviceFor(t, &fakeRepo{}, f).Connect(context.Background(), models.PostmasterCredentials{ClientID: "cid"})
		if xerr != ErrCredentials || f.tokenHit != 0 {
			t.Errorf("xerr=%v tokenHits=%d", xerr, f.tokenHit)
		}
	})

	t.Run("refuses to store without the encryption key", func(t *testing.T) {
		f := newFakeGoogle(t)
		xerr := serviceFor(t, &fakeRepo{noSealer: true}, f).Connect(context.Background(), testCreds)
		if xerr != ErrNoEncryption {
			t.Errorf("xerr=%v", xerr)
		}
	})
}

func TestConnectAndDisconnectAreInstanceScoped(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "cloud")
	repo := &fakeRepo{}
	s := &service{repo: repo}
	if xerr := s.Connect(context.Background(), testCreds); xerr != ErrInstanceScoped {
		t.Errorf("connect on a shared hosted instance: %v", xerr)
	}
	if xerr := s.Disconnect(context.Background()); xerr != ErrInstanceScoped || repo.deleted {
		t.Errorf("disconnect on a shared hosted instance: %v deleted=%v", xerr, repo.deleted)
	}

	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	if xerr := s.Disconnect(context.Background()); xerr != nil || !repo.deleted {
		t.Errorf("self-hosted disconnect: %v deleted=%v", xerr, repo.deleted)
	}
}
