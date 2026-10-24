package jobs

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

func (s *JobsService) HandleMailboxDelete(ctx context.Context, e *models.JobEventMailboxDelete) error {
	if err := s.MailboxRepository.DeleteMailbox(
		ctx,
		e.UserID,
		e.EmailID,
		e.UIDValidity,
	); err != nil {
		CaptureError(e.UserID, e.EmailID, err)
		return err
	}

	return nil
}
