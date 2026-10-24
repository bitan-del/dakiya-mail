package auth

import (
	"context"
	"time"
)

const authEmailSendTimeout = 10 * time.Second

func (s *authService) sendAuthEmail(ctx context.Context, to, subject, message string) error {
	ctx, cancel := context.WithTimeout(ctx, authEmailSendTimeout)
	defer cancel()

	return s.emailNotificationService.Send(ctx, []string{to}, nil, nil, subject, message)
}
