package socket

import (
	"context"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

func getTokenKey(id uuid.UUID) string {
	return "ws_verify:" + id.String()
}

func (s *socketService) saveToken(ctx context.Context, id uuid.UUID, nonce string, expiresAt time.Time) *errx.Error {
	if err := s.cache.SetEx(ctx, getTokenKey(id), nonce, time.Until(expiresAt)).Err(); err != nil {
		sentry.CaptureException(err)
		return errx.InternalError()
	}
	return nil
}
