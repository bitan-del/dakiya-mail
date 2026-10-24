package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
)

// UnengagedMinSends is how many earlier sends a contact needs, with no open,
// click or reply, before the pre-send gate calls it long-unengaged.
const UnengagedMinSends = 3

// CampaignListHistory is how a campaign's audience behaved in earlier mail,
// which CampaignAudience does not carry.
type CampaignListHistory struct {
	// PreviouslyBounced leads bounced in some campaign. Routing skips them at
	// send time, so they shrink the audience rather than hurt the sender.
	PreviouslyBounced int
	// Unengaged leads were sent UnengagedMinSends or more times by other
	// campaigns and never opened, clicked or replied.
	Unengaged int
}

// PreflightRepository reads what the pre-send gate needs beyond the campaign row.
type PreflightRepository interface {
	// Mailboxes returns the given mailboxes with the Advisor's auth state and
	// 30-day send, bounce and complaint counts.
	Mailboxes(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) ([]AdvisorMailbox, error)
	ListHistory(ctx context.Context, orgID, campaignID uuid.UUID) (CampaignListHistory, error)
}

type preflightRepository struct {
	db      *db.DB
	advisor *advisorRepository
}

func NewPreflightRepository(database *db.DB) PreflightRepository {
	return &preflightRepository{db: database, advisor: &advisorRepository{db: database}}
}

func (r *preflightRepository) Mailboxes(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) ([]AdvisorMailbox, error) {
	if len(ids) == 0 {
		return []AdvisorMailbox{}, nil
	}
	return r.advisor.loadMailboxes(ctx, orgID, ids)
}

func (r *preflightRepository) ListHistory(ctx context.Context, orgID, campaignID uuid.UUID) (CampaignListHistory, error) {
	var h CampaignListHistory
	// The bounce test matches the one routing skips on (any campaign), so the
	// count is exactly the leads that will not be sent to.
	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE EXISTS (
				SELECT 1 FROM campaign_contact_progress b
				WHERE b.contact_id = ct.id AND b.bounced_at IS NOT NULL
			)),
			COUNT(*) FILTER (WHERE COALESCE(hist.sends, 0) >= $3 AND COALESCE(hist.engaged, 0) = 0)
		FROM campaign_leads cl
		JOIN contacts ct ON ct.id = cl.contact_id
		JOIN campaigns c ON c.id = cl.campaign_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE p.sent_at IS NOT NULL) AS sends,
				COUNT(*) FILTER (WHERE p.opened_at IS NOT NULL OR p.clicked_at IS NOT NULL OR p.replied_at IS NOT NULL) AS engaged
			FROM campaign_contact_progress p
			WHERE p.contact_id = ct.id AND p.campaign_id <> cl.campaign_id
		) hist ON true
		WHERE cl.campaign_id = $2 AND c.organization_id = $1
	`, orgID, campaignID, UnengagedMinSends).Scan(&h.PreviouslyBounced, &h.Unengaged)
	return h, err
}
