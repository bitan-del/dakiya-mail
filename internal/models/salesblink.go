package models

import (
	"time"

	"github.com/google/uuid"
)

// SalesBlink warmup link. Warmup for a connected mailbox is handed to a
// SalesBlink account instead of the local pool: SalesBlink has a much larger
// pool than a single self-hosted org can build on its own.
//
// The trade is that enrolling a mailbox sends its SMTP/IMAP credentials to
// SalesBlink, so only mailboxes connected with a password can be enrolled.
// OAuth mailboxes (Gmail/Outlook) cannot: their refresh grant is bound to this
// instance's own OAuth app, and SalesBlink's POST /senders is SMTP/IMAP only.

// SalesBlinkDefaultBaseURL is SalesBlink's public API root.
const SalesBlinkDefaultBaseURL = "https://run.salesblink.io/api/public/v1.0.0"

// SalesBlink caps warmup volume per sender in its own schema; mirror the
// bounds so we fail locally with a clear message instead of a 400.
const (
	SalesBlinkMinWarmupPerDay = 1
	SalesBlinkMaxWarmupPerDay = 50
)

// SalesBlinkLink is the single account-level link (one row, id = true).
type SalesBlinkLink struct {
	BaseURL string `json:"base_url"`
	// APIKey is decrypted on read and sealed on write; never serialized.
	APIKey     string `json:"-"`
	AutoEnroll bool   `json:"auto_enroll"`

	WarmupPerDay      int  `json:"warmup_per_day"`
	MaxDailyFrequency int  `json:"max_daily_frequency"`
	RampUpFrequency   int  `json:"ramp_up_frequency"`
	AutoRampUp        bool `json:"auto_ramp_up"`

	ConnectedBy  *uuid.UUID `json:"connected_by"`
	ConnectedAt  time.Time  `json:"connected_at"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastError    string     `json:"last_error"`
}

// SalesBlinkMailbox maps a local mailbox to its remote SalesBlink sender.
type SalesBlinkMailbox struct {
	EmailAccountID uuid.UUID  `json:"email_account_id"`
	SenderID       string     `json:"sender_id"`
	EnrolledAt     time.Time  `json:"enrolled_at"`
	HealthScore    *int       `json:"health_score"`
	LastStatsAt    *time.Time `json:"last_stats_at"`
	LastError      string     `json:"last_error"`
}

// SalesBlinkMailboxRow is one row of the settings table: the local mailbox
// plus its enrollment state, including mailboxes we cannot enroll.
type SalesBlinkMailboxRow struct {
	EmailAccountID uuid.UUID `json:"email_account_id"`
	Email          string    `json:"email"`
	Provider       string    `json:"provider"`
	Status         string    `json:"status"`
	Enrolled       bool      `json:"enrolled"`
	Eligible       bool      `json:"eligible"`
	// IneligibleReason is set when Eligible is false, for the UI to explain why.
	IneligibleReason string     `json:"ineligible_reason,omitempty"`
	SenderID         string     `json:"sender_id,omitempty"`
	HealthScore      *int       `json:"health_score,omitempty"`
	EnrolledAt       *time.Time `json:"enrolled_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
}

// SalesBlinkStatus is what the settings screen renders.
type SalesBlinkStatus struct {
	Connected  bool            `json:"connected"`
	Link       *SalesBlinkLink `json:"link,omitempty"`
	Enrolled   int             `json:"enrolled"`
	Eligible   int             `json:"eligible"`
	Ineligible int             `json:"ineligible"`
}

// SalesBlinkSyncResult reports what a sync pass did.
type SalesBlinkSyncResult struct {
	Enrolled int      `json:"enrolled"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// ─── SalesBlink API wire types ───────────────────────────────────────────

// SalesBlinkSenderRequest is the POST /senders body. Field names are
// SalesBlink's, which mixes camelCase (connection) and snake_case (warmup).
type SalesBlinkSenderRequest struct {
	Email      string `json:"email"`
	SenderName string `json:"senderName,omitempty"`

	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUsername string `json:"smtpUsername,omitempty"`
	SMTPPassword string `json:"smtpPassword"`

	// IMAP is required for warmup: SalesBlink documents warmup_enabled as
	// having no effect on SMTP-only senders, because warmup has to read the
	// inbox to reply to pool mail and pull messages out of spam.
	IMAPHost     string `json:"imapHost,omitempty"`
	IMAPPort     int    `json:"imapPort,omitempty"`
	IMAPUsername string `json:"imapUsername,omitempty"`
	IMAPPassword string `json:"imapPassword,omitempty"`

	WarmupEnabled     bool `json:"warmup_enabled"`
	InboxEnable       bool `json:"inbox_enable"`
	TotalWarmupPerDay int  `json:"total_warmup_per_day,omitempty"`
	AutoRampUpEnabled bool `json:"auto_ramp_up_enabled"`
	RampUpFrequency   int  `json:"ramp_up_frequency,omitempty"`
	MaxDailyFrequency int  `json:"max_daily_frequency,omitempty"`
}

// SalesBlinkSenderResponse is the subset of the sender payload we keep.
// SalesBlink has returned the id under more than one key across versions, so
// accept the common spellings and let the service pick whichever is set.
type SalesBlinkSenderResponse struct {
	ID           string `json:"id"`
	UnderscoreID string `json:"_id"`
	SenderID     string `json:"senderId"`
	Email        string `json:"email"`
}

// ResolveID returns whichever id field SalesBlink populated.
func (r SalesBlinkSenderResponse) ResolveID() string {
	switch {
	case r.ID != "":
		return r.ID
	case r.UnderscoreID != "":
		return r.UnderscoreID
	default:
		return r.SenderID
	}
}

// SalesBlinkHealth is GET /senders/{id}/health.
type SalesBlinkHealth struct {
	Score int `json:"score"`
}
