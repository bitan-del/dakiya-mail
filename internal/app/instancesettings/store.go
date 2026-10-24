package instancesettings

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bitan-del/dakiya-mail/internal/pkg/encrypt"
)

// Store reads and writes the singleton settings row.
type Store interface {
	Get(ctx context.Context) (Document, error)
	Put(ctx context.Context, doc Document, updatedBy *uuid.UUID) error
}

type pgStore struct {
	db  *pgxpool.Pool
	enc *encrypt.Encrypter
}

// NewStore builds the Postgres-backed store. enc (CREDENTIALS_ENCRYPTION_KEY)
// seals the AI API key; with it nil the key cannot be saved or read.
func NewStore(db *pgxpool.Pool, enc *encrypt.Encrypter) Store { return &pgStore{db: db, enc: enc} }

// stored is the at-rest shape: the document with the AI key sealed, which the client-facing Document never carries.
type stored struct {
	Document
	AI storedAI `json:"ai"`
}

type storedAI struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	KeySealed string `json:"api_key_sealed,omitempty"`
}

func decodeDocument(raw []byte, enc *encrypt.Encrypter) (Document, error) {
	s := stored{Document: Defaults()}
	// An empty or partial document keeps the defaults for the keys it omits.
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return Defaults(), err
		}
	}
	doc := s.Document
	doc.AI = AI{Provider: s.AI.Provider, Model: s.AI.Model, BaseURL: s.AI.BaseURL, sealed: s.AI.KeySealed}
	if s.AI.KeySealed != "" {
		if enc == nil {
			log.Print("instance settings: an AI API key is saved but CREDENTIALS_ENCRYPTION_KEY is not set; the key is unavailable")
		} else if plain, err := enc.Decrypt(s.AI.KeySealed); err != nil {
			log.Print("instance settings: the saved AI API key could not be decrypted with CREDENTIALS_ENCRYPTION_KEY; enter it again")
		} else {
			doc.AI.APIKey = plain
		}
	}
	doc.Normalize()
	return doc, nil
}

func encodeDocument(doc Document, enc *encrypt.Encrypter) ([]byte, error) {
	sealed := doc.AI.sealed
	if doc.AI.APIKey != "" {
		if enc == nil {
			return nil, &InvalidError{"Set CREDENTIALS_ENCRYPTION_KEY on the backend before saving an AI API key: it is sealed with that key."}
		}
		var err error
		if sealed, err = enc.Encrypt(doc.AI.APIKey); err != nil {
			return nil, err
		}
	}
	return json.Marshal(stored{
		Document: doc,
		AI:       storedAI{Provider: doc.AI.Provider, Model: doc.AI.Model, BaseURL: doc.AI.BaseURL, KeySealed: sealed},
	})
}

func (s *pgStore) Get(ctx context.Context) (Document, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT doc FROM instance_settings WHERE id = true`).Scan(&raw)
	if err == pgx.ErrNoRows {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), err
	}
	return decodeDocument(raw, s.enc)
}

func (s *pgStore) Put(ctx context.Context, doc Document, updatedBy *uuid.UUID) error {
	raw, err := encodeDocument(doc, s.enc)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO instance_settings (id, doc, updated_by, updated_at)
		VALUES (true, $1::jsonb, $2, NOW())
		ON CONFLICT (id) DO UPDATE
		SET doc = EXCLUDED.doc, updated_by = EXCLUDED.updated_by, updated_at = NOW()
	`, raw, updatedBy)
	return err
}
