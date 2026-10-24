package socket

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/app/token"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cache"
	"github.com/google/uuid"
)

type SocketService interface {
	GenerateWebsocketToken(ctx context.Context, userID uuid.UUID) (string, *errx.Error)
}

type socketService struct {
	cache        *cache.Cache
	tokenService token.TokenService
}

func NewService(cache *cache.Cache, tokenService token.TokenService) SocketService {
	return &socketService{
		cache:        cache,
		tokenService: tokenService,
	}
}
