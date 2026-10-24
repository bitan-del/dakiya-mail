// Package preflight is the pre-send deliverability gate for a campaign: it
// reads the sender domains, mailbox reputation, unsubscribe header, list
// quality and planned volume the Advisor and the launch gate already measure,
// and folds them into one pass, warn or fail verdict before anything sends.
package preflight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/app/advisor"
	"github.com/bitan-del/dakiya-mail/internal/app/listgate"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Shares of the audience, in percent. The role-address warn band is the
// Advisor's; the rest are this gate's own.
const (
	roleShareFail      = 40.0
	bouncedShareWarn   = 5.0
	unengagedShareWarn = 20.0
	unengagedShareFail = 50.0
	// listedMailboxes caps how many offenders a detail line names.
	listedMailboxes = 3
)

// Check is one line of the report. FixHint is empty on a pass.
type Check struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  Status `json:"status"`
	Detail  string `json:"detail"`
	FixHint string `json:"fix_hint"`
}

type Summary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

type Report struct {
	CampaignID  uuid.UUID `json:"campaign_id"`
	Verdict     Status    `json:"verdict"`
	Summary     Summary   `json:"summary"`
	Checks      []Check   `json:"checks"`
	GeneratedAt time.Time `json:"generated_at"`
}

// Input is everything the checks read, already loaded.
type Input struct {
	UnsubscribeHeader bool
	// DailyLimit is the campaign's own daily limit.
	DailyLimit int
	// Mailboxes are the mailboxes that would actually send this campaign.
	Mailboxes []repository.AdvisorMailbox
	Audience  repository.CampaignAudience
	History   repository.CampaignListHistory
}

// Verdict is the worst status across checks.
func Verdict(checks []Check) Status {
	v := StatusPass
	for _, c := range checks {
		switch c.Status {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			v = StatusWarn
		}
	}
	return v
}

// Evaluate runs every check over the input.
func Evaluate(campaignID uuid.UUID, in Input, now time.Time) Report {
	checks := []Check{
		checkDomainAuth(in.Mailboxes),
		checkUnsubscribe(in.UnsubscribeHeader),
		checkComplaints(in.Mailboxes),
		checkBounces(in.Mailboxes),
		checkListValidation(in.Audience),
		checkRoleAddresses(in.Audience),
		checkPreviouslyBounced(in.Audience, in.History),
		checkUnengaged(in.Audience, in.History),
		checkVolume(in),
	}
	var s Summary
	for _, c := range checks {
		switch c.Status {
		case StatusFail:
			s.Fail++
		case StatusWarn:
			s.Warn++
		default:
			s.Pass++
		}
	}
	return Report{CampaignID: campaignID, Verdict: Verdict(checks), Summary: s, Checks: checks, GeneratedAt: now}
}

func checkDomainAuth(mailboxes []repository.AdvisorMailbox) Check {
	c := Check{Key: "domain_auth", Label: "Sending domain authentication"}
	type gap struct {
		domain  string
		missing []string
	}
	var failing, partial []gap
	unchecked := 0
	seen := map[string]bool{}
	for _, m := range mailboxes {
		domain := strings.ToLower(m.Email[strings.LastIndex(m.Email, "@")+1:])
		if seen[domain] {
			continue
		}
		seen[domain] = true
		missing, resolved := advisor.MissingAuthRecords(m)
		switch {
		case !resolved:
			unchecked++
		case contains(missing, "SPF") || contains(missing, "DMARC"):
			// The platform's own send gate fails a domain on exactly these two.
			failing = append(failing, gap{domain, missing})
		case len(missing) > 0:
			partial = append(partial, gap{domain, missing})
		}
	}
	describe := func(gaps []gap) string {
		parts := make([]string, 0, len(gaps))
		for _, g := range gaps {
			parts = append(parts, fmt.Sprintf("%s (%s)", g.domain, strings.Join(g.missing, ", ")))
		}
		return clip(parts)
	}

	switch {
	case len(failing) > 0:
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("Missing records on %s. Google's bulk sender rules need SPF, DKIM and DMARC, and mail without them is filtered on arrival.", describe(failing))
		c.FixHint = "Add the missing DNS records at your domain's registrar, then re-check the domain from the mailbox's settings to clear it right away."
	case len(partial) > 0:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("Missing records on %s. Without them Google's bulk sender rules are not fully met.", describe(partial))
		c.FixHint = "Publish the missing record (DKIM needs a key from your mail provider's admin console), then re-check the domain from the mailbox's settings."
	case unchecked > 0:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("%s not been checked yet, so SPF, DKIM and DMARC are unconfirmed.", plural(unchecked, "sending domain has", "sending domains have"))
		c.FixHint = "Run the authentication check from the mailbox's settings, or wait for the next scheduled check."
	case len(seen) == 0:
		c.Status = StatusPass
		c.Detail = "No sending mailbox to check yet."
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("SPF, DKIM and DMARC are in place on %s.", plural(len(seen), "sending domain", "sending domains"))
	}
	return c
}

func checkUnsubscribe(on bool) Check {
	c := Check{Key: "unsubscribe_header", Label: "One-click unsubscribe"}
	if on {
		c.Status = StatusPass
		c.Detail = "The List-Unsubscribe header is on."
		return c
	}
	c.Status = StatusFail
	c.Detail = "The List-Unsubscribe header is off. Google's bulk sender rules require one-click unsubscribe, and a recipient with no easy way out reaches for the spam button."
	c.FixHint = "Turn on the unsubscribe header in the campaign's preferences. It is a header, not visible copy."
	return c
}

// rated is a mailbox with a measured rate.
type rated struct {
	email string
	rate  float64
	n, of int
}

func (r rated) String() string {
	return fmt.Sprintf("%s %s (%d of %d)", r.email, pctText(r.rate), r.n, r.of)
}

// rateCheck judges a 30-day mailbox rate against its warn and fail bands.
func rateCheck(mailboxes []repository.AdvisorMailbox, floor int, count func(repository.AdvisorMailbox) int, warn, fail float64) (failing, warning []rated, measured int) {
	for _, m := range mailboxes {
		if m.ColdSent30d < floor {
			continue
		}
		measured++
		r := rated{email: m.Email, rate: advisor.Rate(count(m), m.ColdSent30d), n: count(m), of: m.ColdSent30d}
		switch {
		case r.rate >= fail:
			failing = append(failing, r)
		case r.rate >= warn:
			warning = append(warning, r)
		}
	}
	return failing, warning, measured
}

func worstFirst(rs []rated) []string {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].rate > rs[j].rate })
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.String()
	}
	return out
}

func checkComplaints(mailboxes []repository.AdvisorMailbox) Check {
	c := Check{Key: "mailbox_complaints", Label: "Mailbox complaint rate"}
	failing, warning, measured := rateCheck(mailboxes, advisor.MinSendsForComplaintRate,
		func(m repository.AdvisorMailbox) int { return m.Complaints30d },
		advisor.ComplaintRateWarn, advisor.ComplaintRateCritical)
	switch {
	case len(failing) > 0:
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("Over the 0.10%% spam-complaint line Google asks senders to stay under (30 days): %s.", clip(worstFirst(failing)))
		c.FixHint = "Leave these mailboxes out of this campaign, or halve their daily cap and find which campaign drew the complaints before sending more."
	case len(warning) > 0:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("Complaint rate is climbing toward Google's 0.10%% line (30 days): %s.", clip(worstFirst(warning)))
		c.FixHint = "Lower these mailboxes' daily cap and review the copy and list of the campaigns they send for."
	case measured == 0:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("No sender has %d sends in the last 30 days yet, so there is no complaint rate to judge.", advisor.MinSendsForComplaintRate)
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Complaint rate is under 0.03%% on %s.", plural(measured, "measured mailbox", "measured mailboxes"))
	}
	return c
}

func checkBounces(mailboxes []repository.AdvisorMailbox) Check {
	c := Check{Key: "mailbox_bounces", Label: "Mailbox bounce rate"}
	failing, warning, measured := rateCheck(mailboxes, advisor.MinSendsForBounceRate,
		func(m repository.AdvisorMailbox) int { return m.Bounces30d },
		advisor.BounceRateWarn, advisor.BounceRateCritical)
	switch {
	case len(failing) > 0:
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("At or over the 5%% bounce rate where providers put a sender under review (30 days): %s.", clip(worstFirst(failing)))
		c.FixHint = "This is a list problem. Verify the contacts these mailboxes send to and delete every address that hard-bounced before sending more."
	case len(warning) > 0:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("Bounce rate is approaching 5%% (30 days): %s.", clip(worstFirst(warning)))
		c.FixHint = "Verify the list before sending at volume, and remove the domains that bounce repeatedly."
	case measured == 0:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("No sender has %d sends in the last 30 days yet, so there is no bounce rate to judge.", advisor.MinSendsForBounceRate)
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Bounce rate is under 3%% on %s.", plural(measured, "measured mailbox", "measured mailboxes"))
	}
	return c
}

func checkListValidation(a repository.CampaignAudience) Check {
	c := Check{Key: "list_validation", Label: "List validation"}
	if a.Total == 0 {
		c.Status = StatusFail
		c.Detail = "This campaign has no contacts yet."
		c.FixHint = "Add contacts to the campaign before launching."
		return c
	}
	v := listgate.Project(a)
	switch {
	case v.Deliverable == 0:
		c.Status = StatusFail
	case v.Block:
		c.Status = StatusFail
	case v.Warn:
		c.Status = StatusWarn
	default:
		c.Status = StatusPass
	}
	c.Detail = v.Summary
	if c.Status == StatusPass && v.UnverifiedPct >= 1 {
		c.Detail += fmt.Sprintf(" %.0f%% of the list has never been verified.", v.UnverifiedPct)
	}
	if c.Status != StatusPass {
		c.FixHint = v.Remediation
	}
	return c
}

// tooSmall reports an audience under the size where a share means anything.
func tooSmall(a repository.CampaignAudience) bool { return a.Total < listgate.MinAudience }

func checkRoleAddresses(a repository.CampaignAudience) Check {
	c := Check{Key: "list_role_addresses", Label: "Role addresses"}
	if tooSmall(a) {
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Audience of %d is too small to judge.", a.Total)
		return c
	}
	r := advisor.Rate(a.RoleAddress, a.Total)
	switch {
	case r >= roleShareFail:
		c.Status = StatusFail
	case r >= advisor.RoleAddressShareWarn:
		c.Status = StatusWarn
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("%s of the list is shared inboxes like info@ or sales@.", pctText(r))
		return c
	}
	c.Detail = fmt.Sprintf("%s of the list (%d of %d) is shared inboxes like info@ or sales@. They rarely reply and complain more than named people.", pctText(r), a.RoleAddress, a.Total)
	c.FixHint = "Filter role addresses out of the list and find the named person instead."
	return c
}

func checkPreviouslyBounced(a repository.CampaignAudience, h repository.CampaignListHistory) Check {
	c := Check{Key: "list_previously_bounced", Label: "Previously bounced contacts"}
	if tooSmall(a) {
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Audience of %d is too small to judge.", a.Total)
		return c
	}
	r := advisor.Rate(h.PreviouslyBounced, a.Total)
	if r < bouncedShareWarn {
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("%s of the list bounced in earlier mail.", pctText(r))
		return c
	}
	c.Status = StatusWarn
	c.Detail = fmt.Sprintf("%s of the list (%d of %d) bounced in earlier mail. They are skipped at send time, so the campaign reaches fewer people than it shows.", pctText(r), h.PreviouslyBounced, a.Total)
	c.FixHint = "Remove them from the campaign, and re-verify the rest of the list, since a list that carried these addresses often carries more."
	return c
}

func checkUnengaged(a repository.CampaignAudience, h repository.CampaignListHistory) Check {
	c := Check{Key: "list_unengaged", Label: "Long-unengaged contacts"}
	if tooSmall(a) {
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Audience of %d is too small to judge.", a.Total)
		return c
	}
	r := advisor.Rate(h.Unengaged, a.Total)
	switch {
	case r >= unengagedShareFail:
		c.Status = StatusFail
	case r >= unengagedShareWarn:
		c.Status = StatusWarn
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("%s of the list was emailed %d or more times before with no open, click or reply.", pctText(r), repository.UnengagedMinSends)
		return c
	}
	c.Detail = fmt.Sprintf("%s of the list (%d of %d) was emailed %d or more times before with no open, click or reply. Mailing people who never engage teaches inbox providers to filter you.", pctText(r), h.Unengaged, a.Total, repository.UnengagedMinSends)
	c.FixHint = "Leave them out of this send, or move them to a separate low-volume re-engagement campaign."
	return c
}

func checkVolume(in Input) Check {
	c := Check{Key: "daily_volume", Label: "Daily volume vs mailbox caps"}
	n := len(in.Mailboxes)
	if n == 0 {
		c.Status = StatusFail
		c.Detail = "No active mailbox can send this campaign."
		c.FixHint = "Connect a mailbox, or add an active one to the campaign's senders."
		return c
	}
	capacity := 0
	for _, m := range in.Mailboxes {
		capacity += m.CampaignLimit
	}
	if capacity <= 0 {
		c.Status = StatusFail
		c.Detail = "Every sender mailbox has a daily cap of 0, so nothing can send."
		c.FixHint = "Raise the daily cap on at least one sender mailbox."
		return c
	}

	planned := in.DailyLimit
	if d := in.Audience.Deliverable; d > 0 && (planned <= 0 || d < planned) {
		planned = d
	}
	if planned <= 0 {
		planned = capacity
	}
	perMailbox := (min(planned, capacity) + n - 1) / n

	var reasons, hints []string
	if planned > capacity {
		reasons = append(reasons, fmt.Sprintf("Planned volume is %d a day but the %s allow %d a day, and the per-mailbox caps win.", planned, plural(n, "mailbox", "mailboxes"), capacity))
		hints = append(hints, "lower the campaign's daily limit to match, or connect more mailboxes")
	}
	over, fresh := 0, 0
	for _, m := range in.Mailboxes {
		if m.CampaignLimit > advisor.SafeColdCapBand {
			over++
		}
		if m.AgeDays < advisor.NewMailboxDays && min(m.CampaignLimit, perMailbox) > advisor.NewMailboxSafeCap {
			fresh++
		}
	}
	if over > 0 {
		reasons = append(reasons, fmt.Sprintf("%s capped above the %d/day safe band.", plural(over, "mailbox is", "mailboxes are"), advisor.SafeColdCapBand))
		hints = append(hints, fmt.Sprintf("bring those caps back to %d/day unless their complaint and placement numbers justify more", advisor.SafeColdCapBand))
	}
	if fresh > 0 {
		reasons = append(reasons, fmt.Sprintf("%s under %d days old would send over %d a day.", plural(fresh, "mailbox", "mailboxes"), advisor.NewMailboxDays, advisor.NewMailboxSafeCap))
		hints = append(hints, fmt.Sprintf("hold new mailboxes to %d/day and ramp slowly", advisor.NewMailboxSafeCap))
	}

	if len(reasons) == 0 {
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("About %d a day across %s (about %d each), within %d a day of mailbox caps.", min(planned, capacity), plural(n, "mailbox", "mailboxes"), perMailbox, capacity)
		return c
	}
	c.Status = StatusWarn
	c.Detail = strings.Join(reasons, " ")
	c.FixHint = capitalize(strings.Join(hints, "; ")) + "."
	return c
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// clip names the first few offenders and counts the rest.
func clip(parts []string) string {
	if len(parts) <= listedMailboxes {
		return strings.Join(parts, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(parts[:listedMailboxes], ", "), len(parts)-listedMailboxes)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func pctText(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.2f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
