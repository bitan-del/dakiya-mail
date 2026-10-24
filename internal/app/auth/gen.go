package auth

import (
	"context"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/argon2"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

func (s *authService) GenerateLoginSession(ctx context.Context, userID uuid.UUID, code string) (string, *errx.Error) {
	sessID := uuid.New()

	issuedAt := time.Now()
	expiresAt := issuedAt.Add(SessionTTL)

	codeHash, err := argon2.Hash(code)
	if err != nil {
		sentry.CaptureException(err)
		return "", errx.InternalError()
	}

	loginSession := &models.LoginSession{
		CodeHash: codeHash,
		Tries:    0,
	}

	if err := s.saveLoginSession(ctx, sessID, loginSession, expiresAt); err != nil {
		return "", err
	}

	sessionToken, err := s.tokenService.GenerateToken(userID, sessID, "", "", issuedAt, expiresAt)
	if err != nil {
		sentry.CaptureException(err)
		return "", errx.InternalError()
	}

	return sessionToken, nil
}
