package contact

import (
	"context"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/utils/paging"
	"github.com/google/uuid"
)

const segmentPageSize = 500

func validateSegment(req *models.SaveSegmentRequest) *errx.Error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 80 {
		return errx.NewWithIdentifier(errx.BadRequest, "invalid_segment_name", "name must be 1 to 80 characters")
	}
	if len(req.Description) > 500 {
		return errx.New(errx.BadRequest, "description must be at most 500 characters")
	}
	// Sort and pagination are per-view choices, not part of who is in the segment.
	req.Filter.SortBy = ""
	req.Filter.Reverse = false
	return validateLeadFilters(req.Filter)
}

func (s *contactService) countSegment(ctx context.Context, orgID uuid.UUID, filter models.SearchContacts) *int64 {
	res, xerr := s.contactRepository.Search(ctx, orgID.String(), nil, nil, filter, 1)
	if xerr != nil || res == nil {
		return nil
	}
	return res.Pagination.Total
}

func (s *contactService) CreateSegment(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, req *models.SaveSegmentRequest) (*models.Segment, *errx.Error) {
	if xerr := validateSegment(req); xerr != nil {
		return nil, xerr
	}
	seg, xerr := s.contactRepository.CreateSegment(ctx, orgID, userID, req)
	if xerr != nil {
		return nil, xerr
	}
	seg.ContactCount = s.countSegment(ctx, orgID, seg.Filter)
	return seg, nil
}

func (s *contactService) UpdateSegment(ctx context.Context, orgID, id uuid.UUID, req *models.SaveSegmentRequest) (*models.Segment, *errx.Error) {
	if xerr := validateSegment(req); xerr != nil {
		return nil, xerr
	}
	seg, xerr := s.contactRepository.UpdateSegment(ctx, orgID, id, req)
	if xerr != nil {
		return nil, xerr
	}
	seg.ContactCount = s.countSegment(ctx, orgID, seg.Filter)
	return seg, nil
}

func (s *contactService) GetSegment(ctx context.Context, orgID, id uuid.UUID) (*models.Segment, *errx.Error) {
	seg, xerr := s.contactRepository.GetSegment(ctx, orgID, id)
	if xerr != nil {
		return nil, xerr
	}
	seg.ContactCount = s.countSegment(ctx, orgID, seg.Filter)
	return seg, nil
}

func (s *contactService) ListSegments(ctx context.Context, orgID uuid.UUID) ([]models.Segment, *errx.Error) {
	segs, xerr := s.contactRepository.ListSegments(ctx, orgID)
	if xerr != nil {
		return nil, xerr
	}
	for i := range segs {
		segs[i].ContactCount = s.countSegment(ctx, orgID, segs[i].Filter)
	}
	return segs, nil
}

func (s *contactService) DeleteSegment(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	return s.contactRepository.DeleteSegment(ctx, orgID, id)
}

// EnrollSegment adds every contact matching the segment right now to a campaign.
// Contacts already in the campaign are left alone.
func (s *contactService) EnrollSegment(ctx context.Context, userID string, orgID, segmentID, campaignID uuid.UUID) (*models.EnrollSegmentResult, *errx.Error) {
	seg, xerr := s.contactRepository.GetSegment(ctx, orgID, segmentID)
	if xerr != nil {
		return nil, xerr
	}
	ok, xerr := s.contactRepository.CampaignInOrganization(ctx, orgID, campaignID)
	if xerr != nil {
		return nil, xerr
	}
	if !ok {
		return nil, errx.New(errx.NotFound, "campaign not found")
	}

	res := &models.EnrollSegmentResult{}
	campaign := []string{campaignID.String()}
	var cursor *string
	for {
		page, xerr := s.contactRepository.Search(ctx, orgID.String(), nil, cursor, seg.Filter, segmentPageSize)
		if xerr != nil {
			return nil, xerr
		}
		ids := make([]string, 0, len(page.Data))
		uids := make([]uuid.UUID, 0, len(page.Data))
		for _, c := range page.Data {
			if res.Matched >= models.MaxSegmentEnroll {
				res.Capped = true
				break
			}
			ids = append(ids, c.ID.String())
			uids = append(uids, c.ID)
			res.Matched++
		}
		if len(ids) > 0 {
			before, xerr := s.contactRepository.CountCampaignLeads(ctx, campaignID, uids)
			if xerr != nil {
				return nil, xerr
			}
			if _, xerr := s.contactRepository.BulkUpdate(ctx, userID, orgID, &models.BulkEditContactsData{Contacts: ids, AddCampaigns: campaign}); xerr != nil {
				return nil, xerr
			}
			res.Enrolled += len(ids) - before
		}
		if res.Capped || !page.Pagination.HasMore || page.Pagination.NextCursor == nil {
			break
		}
		id, err := paging.DecodeUUID(*page.Pagination.NextCursor)
		if err != nil {
			break
		}
		next := id.String()
		cursor = &next
	}

	s.publishContactsReload(ctx, userID, "contacts:bulk_update")
	s.wakeCampaigns(ctx, orgID, campaign)
	return res, nil
}
