package encryptedkeys

import (
	"fmt"
	"os"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/db"
)

// Deps bundles the dependencies the factory might need. Each is only consulted
// when the matching provider is selected — callers may pass nil for any
// dependency they don't have available (e.g. workers pass nil for db).
type Deps struct {
	DB *db.DB
}

// FromEnv constructs the active encrypted-keys store from environment vars.
//
//	ENCRYPTED_KEYS_PROVIDER=postgres   (default; backend/consumer durable store)
//	ENCRYPTED_KEYS_PROVIDER=http       (worker-side: calls backend over HTTPS)
//	  ENCRYPTED_KEYS_BACKEND_URL       (required, e.g. https://api.dakiya.example.com)
//	  ENCRYPTED_KEYS_WORKER_TOKEN      (required, bearer token)
//
// fallback selects a default provider when ENCRYPTED_KEYS_PROVIDER is unset.
// Backend processes should pass "postgres"; workers should pass "http".
func FromEnv(deps Deps, fallback string) (Store, error) {
	provider := os.Getenv("ENCRYPTED_KEYS_PROVIDER")
	if provider == "" {
		provider = fallback
	}
	switch provider {
	case "postgres", "pg":
		if deps.DB == nil {
			return nil, fmt.Errorf("encryptedkeys: postgres provider requires a *db.DB")
		}
		return NewPostgres(deps.DB), nil
	case "http":
		baseURL := os.Getenv("ENCRYPTED_KEYS_BACKEND_URL")
		token := os.Getenv("ENCRYPTED_KEYS_WORKER_TOKEN")
		return NewHTTP(baseURL, token)
	default:
		return nil, fmt.Errorf("encryptedkeys: unknown ENCRYPTED_KEYS_PROVIDER %q (want: postgres, http)", provider)
	}
}

// Compile-time interface assertions.
var (
	_ Store = (*PostgresStore)(nil)
	_ Store = (*HTTPStore)(nil)
)
