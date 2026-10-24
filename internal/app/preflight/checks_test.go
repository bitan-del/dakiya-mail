package preflight

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

// healthy is a sender that passes every mailbox-level check.
func healthy(email string) repository.AdvisorMailbox {
	return repository.AdvisorMailbox{
		Email: email, AgeDays: 120, CampaignLimit: 40,
		AuthState: models.AuthStatePassing, AuthSPF: true, AuthDKIM: true, AuthDMARC: true,
		ColdSent30d: 1000,
	}
}

func with(m repository.AdvisorMailbox, f func(*repository.AdvisorMailbox)) repository.AdvisorMailbox {
	f(&m)
	return m
}

func cleanAudience(n int) repository.CampaignAudience {
	return repository.CampaignAudience{Total: n, Deliverable: n}
}

func healthyInput() Input {
	return Input{
		UnsubscribeHeader: true,
		DailyLimit:        60,
		Mailboxes:         []repository.AdvisorMailbox{healthy("a@one.io"), healthy("b@two.io")},
		Audience:          cleanAudience(1000),
	}
}

func wantStatus(t *testing.T, name string, got Check, want Status) {
	t.Helper()
	if got.Status != want {
		t.Errorf("%s: status = %s, want %s (detail: %s)", name, got.Status, want, got.Detail)
	}
	if got.Status != StatusPass && got.FixHint == "" {
		t.Errorf("%s: a %s check must say how to fix it", name, got.Status)
	}
	if got.Detail == "" {
		t.Errorf("%s: a check must say what it found", name)
	}
}

func TestVerdictIsTheWorstStatus(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		want   Status
	}{
		{"no checks", nil, StatusPass},
		{"all pass", []Check{{Status: StatusPass}, {Status: StatusPass}}, StatusPass},
		{"a warn", []Check{{Status: StatusPass}, {Status: StatusWarn}}, StatusWarn},
		{"a fail", []Check{{Status: StatusPass}, {Status: StatusFail}}, StatusFail},
		{"fail beats warn in either order", []Check{{Status: StatusFail}, {Status: StatusWarn}}, StatusFail},
		{"warn then fail", []Check{{Status: StatusWarn}, {Status: StatusFail}}, StatusFail},
	}
	for _, tc := range cases {
		if got := Verdict(tc.checks); got != tc.want {
			t.Errorf("%s: verdict = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestEvaluateOnAHealthyCampaignPassesEverything(t *testing.T) {
	id := uuid.New()
	r := Evaluate(id, healthyInput(), time.Unix(0, 0))
	if r.Verdict != StatusPass || r.Summary.Fail != 0 || r.Summary.Warn != 0 {
		t.Fatalf("a healthy campaign should pass cleanly: %+v", r.Summary)
	}
	if r.CampaignID != id || r.Summary.Pass != len(r.Checks) {
		t.Errorf("summary %+v does not account for %d checks", r.Summary, len(r.Checks))
	}
	keys := map[string]bool{}
	for _, c := range r.Checks {
		if c.Key == "" || c.Label == "" || c.Detail == "" {
			t.Errorf("incomplete check: %+v", c)
		}
		if keys[c.Key] {
			t.Errorf("duplicate check key %q", c.Key)
		}
		keys[c.Key] = true
	}
}

func TestEvaluateCountsEachStatusAndTakesTheWorstVerdict(t *testing.T) {
	in := healthyInput()
	in.UnsubscribeHeader = false                               // fail
	in.Audience.RoleAddress = 150                              // warn at 15%
	in.Mailboxes = append(in.Mailboxes, healthy("c@three.io")) // still fine
	r := Evaluate(uuid.New(), in, time.Now())
	if r.Verdict != StatusFail || r.Summary.Fail != 1 || r.Summary.Warn != 1 {
		t.Fatalf("summary = %+v, verdict = %s", r.Summary, r.Verdict)
	}
	if r.Summary.Pass+r.Summary.Warn+r.Summary.Fail != len(r.Checks) {
		t.Errorf("summary %+v does not add up to %d checks", r.Summary, len(r.Checks))
	}
}

func TestDomainAuthBoundaries(t *testing.T) {
	broken := func(email string, f func(*repository.AdvisorMailbox)) repository.AdvisorMailbox {
		return with(healthy(email), func(m *repository.AdvisorMailbox) {
			m.AuthState = models.AuthStateFailing
			f(m)
		})
	}
	cases := []struct {
		name string
		in   []repository.AdvisorMailbox
		want Status
	}{
		{"all three present", []repository.AdvisorMailbox{healthy("a@x.io")}, StatusPass},
		{"no sender", nil, StatusPass},
		{"never checked", []repository.AdvisorMailbox{with(healthy("a@x.io"), func(m *repository.AdvisorMailbox) { m.AuthState = models.AuthStateUnknown })}, StatusWarn},
		{"empty state", []repository.AdvisorMailbox{with(healthy("a@x.io"), func(m *repository.AdvisorMailbox) { m.AuthState = "" })}, StatusWarn},
		{"DKIM only missing", []repository.AdvisorMailbox{with(healthy("a@x.io"), func(m *repository.AdvisorMailbox) { m.AuthDKIM = false })}, StatusWarn},
		{"SPF missing", []repository.AdvisorMailbox{broken("a@x.io", func(m *repository.AdvisorMailbox) { m.AuthSPF = false })}, StatusFail},
		{"DMARC missing", []repository.AdvisorMailbox{broken("a@x.io", func(m *repository.AdvisorMailbox) { m.AuthDMARC = false })}, StatusFail},
		{"one bad domain among good ones", []repository.AdvisorMailbox{healthy("a@x.io"), broken("b@y.io", func(m *repository.AdvisorMailbox) { m.AuthSPF = false })}, StatusFail},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkDomainAuth(tc.in), tc.want)
	}
}

func TestDomainAuthNamesEachDomainOnce(t *testing.T) {
	bad := func(email string) repository.AdvisorMailbox {
		return with(healthy(email), func(m *repository.AdvisorMailbox) {
			m.AuthState = models.AuthStateFailing
			m.AuthSPF = false
		})
	}
	c := checkDomainAuth([]repository.AdvisorMailbox{bad("a@x.io"), bad("b@X.io"), bad("c@y.io")})
	if n := strings.Count(strings.ToLower(c.Detail), "x.io"); n != 1 {
		t.Errorf("x.io named %d times in %q, want once", n, c.Detail)
	}
	if !strings.Contains(c.Detail, "y.io") {
		t.Errorf("second domain missing from %q", c.Detail)
	}
}

func TestUnsubscribeBoundaries(t *testing.T) {
	wantStatus(t, "on", checkUnsubscribe(true), StatusPass)
	wantStatus(t, "off", checkUnsubscribe(false), StatusFail)
}

func TestComplaintBoundaries(t *testing.T) {
	// 10,000 sends puts each complaint at 0.01%, so the bands are exact.
	m := func(sends, complaints int) []repository.AdvisorMailbox {
		return []repository.AdvisorMailbox{with(healthy("a@x.io"), func(b *repository.AdvisorMailbox) {
			b.ColdSent30d, b.Complaints30d = sends, complaints
		})}
	}
	cases := []struct {
		name            string
		sends, complain int
		want            Status
	}{
		{"under the sample floor, every send a complaint", 99, 99, StatusPass},
		{"at the floor and clean", 100, 0, StatusPass},
		{"just under the warn band", 10000, 2, StatusPass},
		{"at the warn band", 10000, 3, StatusWarn},
		{"just under the fail band", 10000, 9, StatusWarn},
		{"at the fail band", 10000, 10, StatusFail},
		{"far over", 1000, 20, StatusFail},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkComplaints(m(tc.sends, tc.complain)), tc.want)
	}
}

func TestBounceBoundaries(t *testing.T) {
	m := func(sends, bounces int) []repository.AdvisorMailbox {
		return []repository.AdvisorMailbox{with(healthy("a@x.io"), func(b *repository.AdvisorMailbox) {
			b.ColdSent30d, b.Bounces30d = sends, bounces
		})}
	}
	cases := []struct {
		name           string
		sends, bounces int
		want           Status
	}{
		{"under the sample floor, every send bounced", 49, 49, StatusPass},
		{"clean", 1000, 0, StatusPass},
		{"just under the warn band", 100, 2, StatusPass},
		{"at the warn band", 100, 3, StatusWarn},
		{"just under the fail band", 100, 4, StatusWarn},
		{"at the fail band", 100, 5, StatusFail},
		{"well over", 100, 30, StatusFail},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkBounces(m(tc.sends, tc.bounces)), tc.want)
	}
}

func TestRateChecksNameTheWorstMailboxFirst(t *testing.T) {
	mild := with(healthy("mild@x.io"), func(b *repository.AdvisorMailbox) { b.Bounces30d = 40 })
	bad := with(healthy("bad@x.io"), func(b *repository.AdvisorMailbox) { b.Bounces30d = 200 })
	c := checkBounces([]repository.AdvisorMailbox{mild, bad})
	if c.Status != StatusFail || !strings.Contains(c.Detail, "bad@x.io") {
		t.Fatalf("expected the failing mailbox named: %+v", c)
	}
	if strings.Contains(c.Detail, "mild@x.io") {
		t.Errorf("a mailbox below the fail band should not be listed as failing: %s", c.Detail)
	}
}

func TestListValidationBoundaries(t *testing.T) {
	cases := []struct {
		name string
		a    repository.CampaignAudience
		want Status
	}{
		{"no contacts", repository.CampaignAudience{}, StatusFail},
		{"every lead suppressed", repository.CampaignAudience{Total: 100, Suppressed: 100}, StatusFail},
		{"verified and clean", repository.CampaignAudience{Total: 1000, Deliverable: 1000}, StatusPass},
		{"under the warn band", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Invalid: 19}, StatusPass},
		{"at the warn band", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Invalid: 20}, StatusWarn},
		{"under the fail band", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Invalid: 39}, StatusWarn},
		{"at the fail band", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Invalid: 40}, StatusFail},
		{"never verified at all", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Unknown: 1000}, StatusWarn},
		{"never verified below the advise share", repository.CampaignAudience{Total: 1000, Deliverable: 1000, Unknown: 499}, StatusPass},
		{"too small to judge, all invalid", repository.CampaignAudience{Total: 10, Deliverable: 10, Invalid: 10}, StatusPass},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkListValidation(tc.a), tc.want)
	}
}

func TestListValidationMentionsTheUnverifiedShareOnAPass(t *testing.T) {
	c := checkListValidation(repository.CampaignAudience{Total: 1000, Deliverable: 1000, Unknown: 300})
	if c.Status != StatusPass || !strings.Contains(c.Detail, "30% of the list has never been verified") {
		t.Errorf("a passing list should still report what is unverified: %+v", c)
	}
}

func TestRoleAddressBoundaries(t *testing.T) {
	a := func(total, role int) repository.CampaignAudience {
		return repository.CampaignAudience{Total: total, Deliverable: total, RoleAddress: role}
	}
	cases := []struct {
		name        string
		total, role int
		want        Status
	}{
		{"under the warn share", 100, 14, StatusPass},
		{"at the warn share", 100, 15, StatusWarn},
		{"under the fail share", 100, 39, StatusWarn},
		{"at the fail share", 100, 40, StatusFail},
		{"too small to judge", 49, 49, StatusPass},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkRoleAddresses(a(tc.total, tc.role)), tc.want)
	}
}

func TestPreviouslyBouncedBoundaries(t *testing.T) {
	a := cleanAudience(100)
	cases := []struct {
		name    string
		bounced int
		a       repository.CampaignAudience
		want    Status
	}{
		{"none", 0, a, StatusPass},
		{"under the warn share", 4, a, StatusPass},
		{"at the warn share", 5, a, StatusWarn},
		{"all of them", 100, a, StatusWarn},
		{"too small to judge", 20, cleanAudience(20), StatusPass},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkPreviouslyBounced(tc.a, repository.CampaignListHistory{PreviouslyBounced: tc.bounced}), tc.want)
	}
}

func TestUnengagedBoundaries(t *testing.T) {
	a := cleanAudience(100)
	cases := []struct {
		name      string
		unengaged int
		a         repository.CampaignAudience
		want      Status
	}{
		{"none", 0, a, StatusPass},
		{"under the warn share", 19, a, StatusPass},
		{"at the warn share", 20, a, StatusWarn},
		{"under the fail share", 49, a, StatusWarn},
		{"at the fail share", 50, a, StatusFail},
		{"too small to judge", 20, cleanAudience(20), StatusPass},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkUnengaged(tc.a, repository.CampaignListHistory{Unengaged: tc.unengaged}), tc.want)
	}
}

func TestVolumeBoundaries(t *testing.T) {
	aged := func(cap int) repository.AdvisorMailbox {
		return with(healthy("a@x.io"), func(m *repository.AdvisorMailbox) { m.CampaignLimit = cap })
	}
	fresh := func(cap int) repository.AdvisorMailbox {
		return with(healthy("n@x.io"), func(m *repository.AdvisorMailbox) { m.CampaignLimit, m.AgeDays = cap, 5 })
	}
	cases := []struct {
		name      string
		mailboxes []repository.AdvisorMailbox
		daily     int
		audience  repository.CampaignAudience
		want      Status
	}{
		{"no mailbox", nil, 50, cleanAudience(500), StatusFail},
		{"every cap is zero", []repository.AdvisorMailbox{aged(0), aged(0)}, 50, cleanAudience(500), StatusFail},
		{"within caps", []repository.AdvisorMailbox{aged(40), aged(40)}, 60, cleanAudience(500), StatusPass},
		{"exactly at capacity", []repository.AdvisorMailbox{aged(40), aged(40)}, 80, cleanAudience(500), StatusPass},
		{"one over capacity", []repository.AdvisorMailbox{aged(40), aged(40)}, 81, cleanAudience(500), StatusWarn},
		{"a small list lowers the plan under capacity", []repository.AdvisorMailbox{aged(40), aged(40)}, 500, cleanAudience(60), StatusPass},
		{"cap above the safe band", []repository.AdvisorMailbox{aged(51)}, 40, cleanAudience(500), StatusWarn},
		{"cap at the safe band", []repository.AdvisorMailbox{aged(50)}, 40, cleanAudience(500), StatusPass},
		{"new mailbox over the new-mailbox cap", []repository.AdvisorMailbox{fresh(30)}, 30, cleanAudience(500), StatusWarn},
		{"new mailbox at the new-mailbox cap", []repository.AdvisorMailbox{fresh(20)}, 20, cleanAudience(500), StatusPass},
		{"new mailbox with a high cap but a light plan", []repository.AdvisorMailbox{fresh(50)}, 10, cleanAudience(500), StatusPass},
		{"no campaign limit set", []repository.AdvisorMailbox{aged(40)}, 0, repository.CampaignAudience{}, StatusPass},
	}
	for _, tc := range cases {
		wantStatus(t, tc.name, checkVolume(Input{Mailboxes: tc.mailboxes, DailyLimit: tc.daily, Audience: tc.audience}), tc.want)
	}
}

func TestVolumeReportsEveryReasonItWarnsOn(t *testing.T) {
	m := with(healthy("n@x.io"), func(m *repository.AdvisorMailbox) { m.CampaignLimit, m.AgeDays = 60, 3 })
	c := checkVolume(Input{Mailboxes: []repository.AdvisorMailbox{m}, DailyLimit: 200, Audience: cleanAudience(500)})
	for _, want := range []string{"per-mailbox caps win", "safe band", "days old"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail %q is missing %q", c.Detail, want)
		}
	}
}
