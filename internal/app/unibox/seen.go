package unibox

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

func (s *uniboxService) MarkSeen(ctx context.Context, userID, emailID uuid.UUID, seen bool) *errx.Error {
	if err := s.uniboxRepository.MarkSeen(ctx, userID, emailID, seen); err != nil {
		sentry.CaptureException(err)
		return errx.InternalError()
	}

	return nil
}

func (s *uniboxService) MarkSeenBulk(ctx context.Context, orgID uuid.UUID, data *models.MarkSeen) (*models.MarkSeen, *errx.Error) {
	if len(data.EmailIDs) > 500 {
		return nil, errx.ErrSeenMax
	}

	if err := s.uniboxRepository.MarkSeenBulk(ctx, orgID, data.EmailIDs, data.Seen); err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}

	return data, nil
}
