package user

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cache"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/google/uuid"
)

type UserService interface {
	SaveUser(ctx context.Context, user *models.User) *errx.Error
	GetUser(ctx context.Context, userID uuid.UUID) (*models.User, *errx.Error)
	CompleteOnboarding(ctx context.Context, userID uuid.UUID, firstName, lastName, referralSource, role, teamSize string) *errx.Error
	UpdateProfile(ctx context.Context, userID uuid.UUID, firstName, lastName string) *errx.Error
	UpdateUndoSendSeconds(ctx context.Context, userID uuid.UUID, seconds int) *errx.Error
}

type userService struct {
	cache          *cache.Cache
	userRepository repository.UserRepository
}

func NewService(userRepository repository.UserRepository, cache *cache.Cache) UserService {
	return &userService{
		userRepository: userRepository,
		cache:          cache,
	}
}
