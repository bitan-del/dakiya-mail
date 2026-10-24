package preflight

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

type Service interface {
	// Run measures one campaign as it is now. It reads only; nothing is stored or sent.
	Run(ctx context.Context, orgID, campaignID uuid.UUID) (*Report, *errx.Error)
}

type service struct {
	campaigns repository.CampaignRepository
	emails    repository.EmailRepository
	audience  repository.CampaignAudienceRepository
	repo      repository.PreflightRepository
}

func NewService(campaigns repository.CampaignRepository, emails repository.EmailRepository, audience repository.CampaignAudienceRepository, repo repository.PreflightRepository) Service {
	return &service{campaigns: campaigns, emails: emails, audience: audience, repo: repo}
}

func (s *service) Run(ctx context.Context, orgID, campaignID uuid.UUID) (*Report, *errx.Error) {
	campaign, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		log.Warn().Err(err).Str("campaign_id", campaignID.String()).Msg("preflight: could not load the campaign")
		return nil, errx.InternalError()
	}
	// Another organization's campaign reads as missing, never as forbidden.
	if campaign == nil || campaign.OrganizationID == nil || *campaign.OrganizationID != orgID {
		return nil, errx.ErrNotFound
	}

	ids, xerr := s.senderIDs(ctx, campaign)
	if xerr != nil {
		return nil, xerr
	}
	mailboxes, err := s.repo.Mailboxes(ctx, orgID, ids)
	if err != nil {
		log.Warn().Err(err).Str("campaign_id", campaignID.String()).Msg("preflight: could not read the sender mailboxes")
		return nil, errx.InternalError()
	}
	audience, err := s.audience.GetCampaignAudience(ctx, orgID, campaignID)
	if err != nil {
		log.Warn().Err(err).Str("campaign_id", campaignID.String()).Msg("preflight: could not measure the audience")
		return nil, errx.InternalError()
	}
	history, err := s.repo.ListHistory(ctx, orgID, campaignID)
	if err != nil {
		log.Warn().Err(err).Str("campaign_id", campaignID.String()).Msg("preflight: could not read the audience history")
		return nil, errx.InternalError()
	}

	report := Evaluate(campaignID, Input{
		UnsubscribeHeader: campaign.UnsubscribeHeader,
		DailyLimit:        campaign.DailyLimit,
		Mailboxes:         mailboxes,
		Audience:          audience,
		History:           history,
	}, time.Now().UTC())
	return &report, nil
}

// senderIDs is the pool the scheduler sends from: the explicit senders plus the
// tag-matched mailboxes, or every active mailbox when the campaign names neither.
func (s *service) senderIDs(ctx context.Context, campaign *models.Campaign) ([]uuid.UUID, *errx.Error) {
	scope := repository.NewAccountScope(campaign.OrganizationID)
	explicit, xerr := s.emails.GetByCampaignSenders(ctx, scope, campaign.ID)
	if xerr != nil {
		return nil, xerr
	}
	seen := map[uuid.UUID]bool{}
	ids := []uuid.UUID{}
	add := func(id uuid.UUID) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, e := range explicit {
		add(e.Account.ID)
	}
	if len(campaign.EmailTags) > 0 {
		tagged, xerr := s.emails.GetByTags(ctx, scope, campaign.EmailTags)
		if xerr != nil {
			return nil, xerr
		}
		for _, e := range tagged {
			add(e.ID)
		}
	}
	if len(explicit) == 0 && len(campaign.EmailTags) == 0 {
		all, xerr := s.emails.GetAllActiveInScope(ctx, scope)
		if xerr != nil {
			return nil, xerr
		}
		for _, e := range all {
			add(e.ID)
		}
	}
	return ids, nil
}
