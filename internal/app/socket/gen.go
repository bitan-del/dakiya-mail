package socket

import (
	"context"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/pkg/crypt"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

func (s *socketService) GenerateWebsocketToken(ctx context.Context, userID uuid.UUID) (string, *errx.Error) {
	issuedAt := time.Now()
	expiresAt := issuedAt.Add(SocketTTL)
	id := uuid.New()
	nonce, err := crypt.Nonce()
	if err != nil {
		sentry.CaptureException(err)
		return "", errx.InternalError()
	}

	wsToken, err := s.tokenService.GenerateToken(userID, id, "", nonce, issuedAt, expiresAt)
	if err != nil {
		sentry.CaptureException(err)
		return "", errx.InternalError()
	}

	if err := s.saveToken(ctx, id, nonce, expiresAt); err != nil {
		return "", err
	}

	return wsToken, nil
}
