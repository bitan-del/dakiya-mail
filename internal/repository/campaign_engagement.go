package repository

import (
	"fmt"
	"math"
	"time"
)

// Score weights: a reply says more than a click, a click more than an open. Each term halves every engagementHalfLife.
const (
	engagementReplyWeight = 10.0
	engagementClickWeight = 4.0
	engagementOpenWeight  = 1.0
	engagementHalfLife    = 30 * 24 * time.Hour
)

// NewLeadPolicy is the campaign's engagement rules for contacts who have not started its sequence yet.
type NewLeadPolicy struct {
	// EngagedFirst offers new leads in engagement order instead of the campaign's contact order.
	EngagedFirst bool
	// SunsetAfter > 0 skips new leads with that many campaign emails and no engagement; 0 is off.
	SunsetAfter int
}

func (p NewLeadPolicy) active() bool { return p.EngagedFirst || p.SunsetAfter > 0 }

// engagementSignals is what a contact has done with the organization's campaign mail, from campaign_contact_progress.
type engagementSignals struct {
	Sends int
	// LastOpenAt is a human open only: automated fetches (privacy proxies, scanners) are flagged opened_machine.
	LastReplyAt, LastClickAt, LastOpenAt *time.Time
}

func (e engagementSignals) engaged() bool {
	return e.LastReplyAt != nil || e.LastClickAt != nil || e.LastOpenAt != nil
}

// lastEngagedAt is the most recent reply, click or human open.
func (e engagementSignals) lastEngagedAt() *time.Time {
	var last *time.Time
	for _, t := range []*time.Time{e.LastReplyAt, e.LastClickAt, e.LastOpenAt} {
		if t != nil && (last == nil || t.After(*last)) {
			last = t
		}
	}
	return last
}

// score is the recency-decayed weight of the latest reply, click and open; 0 when the contact never engaged.
func (e engagementSignals) score(now time.Time) float64 {
	decayed := func(at *time.Time, weight float64) float64 {
		if at == nil {
			return 0
		}
		age := max(now.Sub(*at), 0)
		return weight * math.Exp2(-float64(age)/float64(engagementHalfLife))
	}
	return decayed(e.LastReplyAt, engagementReplyWeight) +
		decayed(e.LastClickAt, engagementClickWeight) +
		decayed(e.LastOpenAt, engagementOpenWeight)
}

// sunset reports whether the contact has had `after` or more campaign emails and never engaged; after <= 0 is off.
func (e engagementSignals) sunset(after int) bool {
	return after > 0 && e.Sends >= after && !e.engaged()
}

// tier groups new leads for engaged-first ordering: engaged, then never contacted, then contacted without engaging.
func (e engagementSignals) tier() int {
	switch {
	case e.engaged():
		return 0
	case e.Sends == 0:
		return 1
	default:
		return 2
	}
}

// dueNewLead is a new lead whose first step is due, with the signals that rank it.
type dueNewLead struct {
	pair ContactSequencePair
	eng  engagementSignals
}

// pickEngagedFirst returns the best due new lead: by tier, then score (engaged tier only), ties keep list order.
func pickEngagedFirst(leads []dueNewLead, now time.Time) *dueNewLead {
	var best *dueNewLead
	var bestScore float64
	for i := range leads {
		l := &leads[i]
		score := l.eng.score(now)
		if best == nil || l.eng.tier() < best.eng.tier() || (l.eng.tier() == best.eng.tier() && score > bestScore) {
			best, bestScore = l, score
		}
	}
	return best
}

// engagementLateral reads a new lead's org-wide campaign history; the lp gate leaves leads already in the sequence unread.
const engagementLateral = `
		LEFT JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE ep.sent_at IS NOT NULL AND es.kind = 'email') AS sends,
			       MAX(ep.replied_at) AS last_reply_at,
			       MAX(ep.clicked_at) AS last_click_at,
			       MAX(ep.opened_at) FILTER (WHERE NOT ep.opened_machine) AS last_open_at
			FROM campaign_contact_progress ep
			LEFT JOIN sequences es ON es.id = ep.sequence_id
			WHERE ep.contact_id = cl.contact_id AND lp.sequence_id IS NULL
		) eng ON true`

// sunsetClause is the SQL form of engagementSignals.sunset for a contact (aliased c) about to start campaign cp.
func sunsetClause(cp string) string {
	return fmt.Sprintf(
		"(COALESCE((SELECT sc.sunset_after_sends FROM campaigns sc WHERE sc.id = %[1]s), 0) > 0 "+
			"AND NOT EXISTS (SELECT 1 FROM campaign_contact_progress np WHERE np.campaign_id = %[1]s AND np.contact_id = c.id AND np.sent_at IS NOT NULL) "+
			"AND (SELECT COUNT(*) FROM campaign_contact_progress sp LEFT JOIN sequences sq ON sq.id = sp.sequence_id "+
			"WHERE sp.contact_id = c.id AND sp.sent_at IS NOT NULL AND sq.kind = 'email') "+
			">= (SELECT sc.sunset_after_sends FROM campaigns sc WHERE sc.id = %[1]s) "+
			"AND NOT EXISTS (SELECT 1 FROM campaign_contact_progress gp WHERE gp.contact_id = c.id "+
			"AND (gp.replied_at IS NOT NULL OR gp.clicked_at IS NOT NULL OR (gp.opened_at IS NOT NULL AND NOT gp.opened_machine))))",
		cp,
	)
}
