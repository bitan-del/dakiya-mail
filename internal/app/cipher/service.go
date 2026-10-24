package cipher

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cache"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/encryptedkeys"
	"github.com/bitan-del/dakiya-mail/internal/infrastructure/kms"
	"github.com/google/uuid"
)

type CipherService interface {
	Cipher(ctx context.Context, orgID uuid.UUID) (*Cipher, error)
}

type cipherService struct {
	encryptedKeys encryptedkeys.Store
	cache         *cache.Cache
	kms           kms.Provider
}

func NewService(kms kms.Provider, cache *cache.Cache, encryptedKeys encryptedkeys.Store) CipherService {
	return &cipherService{
		kms:           kms,
		cache:         cache,
		encryptedKeys: encryptedKeys,
	}
}
