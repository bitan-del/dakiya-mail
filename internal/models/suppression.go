package models

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sources a suppression can carry besides the deliverability event types.
const (
	SuppressionSourceManual DeliverabilityEventType = "manual"
	SuppressionSourceImport DeliverabilityEventType = "import"
	SuppressionSourceHashed DeliverabilityEventType = "hashed_list"
)

type SuppressionHashAlgo string

const (
	SuppressionHashMD5    SuppressionHashAlgo = "md5"
	SuppressionHashSHA256 SuppressionHashAlgo = "sha256"
)

// HexLen is the length of a valid lowercase hex digest for the algorithm.
func (a SuppressionHashAlgo) HexLen() int {
	switch a {
	case SuppressionHashMD5:
		return 32
	case SuppressionHashSHA256:
		return 64
	}
	return 0
}

// NormalizeSuppressionEmail is the form every suppression digest is taken over.
func NormalizeSuppressionEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// SuppressionDigests returns the md5 and sha256 hex digests of the normalized address.
func SuppressionDigests(email string) (md5Hex, sha256Hex string) {
	n := NormalizeSuppressionEmail(email)
	m := md5.Sum([]byte(n))
	s := sha256.Sum256([]byte(n))
	return hex.EncodeToString(m[:]), hex.EncodeToString(s[:])
}

type SuppressionListResult struct {
	Data       []SuppressedRecipient `json:"data"`
	Pagination Pagination            `json:"pagination"`
}

type AddSuppressionsRequest struct {
	Emails    []string   `json:"emails"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type RemoveSuppressionsRequest struct {
	Emails []string `json:"emails"`
}

type SuppressionImportResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
	Invalid int `json:"invalid"`
}

type SuppressionHashList struct {
	ListName  string              `json:"list_name"`
	Algo      SuppressionHashAlgo `json:"algo"`
	Count     int64               `json:"count"`
	CreatedAt time.Time           `json:"created_at"`
}

type SuppressedHash struct {
	OrganizationID uuid.UUID
	Algo           SuppressionHashAlgo
	Hash           string
	ListName       string
}
