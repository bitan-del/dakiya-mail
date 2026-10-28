package advanced

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
)

func (s *service) RecordTransactionalMessage(ctx context.Context, orgID, taskID uuid.UUID, templateID *uuid.UUID, recipient string, trackIDs []string) *errx.Error {
	if err := s.repo.RecordTransactionalMessage(ctx, orgID, taskID, templateID, recipient, trackIDs); err != nil {
		return toErrx(err)
	}
	return nil
}

func (s *service) GetTransactionalMessage(ctx context.Context, orgID, taskID uuid.UUID) (*models.TransactionalMessage, *errx.Error) {
	m, err := s.repo.GetTransactionalMessage(ctx, orgID, taskID)
	if err != nil {
		return nil, toErrx(err)
	}
	if m == nil {
		return nil, errx.New(errx.NotFound, "transactional message not found")
	}
	return m, nil
}
