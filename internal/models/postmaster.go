package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AuditEntityPostmaster is the Google Postmaster credential connect/disconnect.
// It rides the audit spine so every member's deliverability page refreshes.
const AuditEntityPostmaster AuditEntityType = "postmaster"

// Gmail's published user-reported spam rate bands: stay under warn, never reach critical.
const (
	PostmasterSpamWarn     = 0.001
	PostmasterSpamCritical = 0.003
)

// PostmasterDay is Gmail's view of one sending domain on one day. Every ratio
// is 0..1 and nil when Gmail reported nothing: low-volume domains have no data.
type PostmasterDay struct {
	Date             string   `json:"date"` // YYYY-MM-DD
	DomainReputation *string  `json:"domain_reputation"`
	SpamRate         *float64 `json:"spam_rate"`
	SPFRatio         *float64 `json:"spf_ratio"`
	DKIMRatio        *float64 `json:"dkim_ratio"`
	DMARCRatio       *float64 `json:"dmarc_ratio"`
}

// PostmasterSummary aggregates the days that carried data inside the window.
type PostmasterSummary struct {
	DaysWithData     int      `json:"days_with_data"`
	LatestDate       *string  `json:"latest_date"`
	LatestReputation *string  `json:"latest_reputation"`
	SpamRateAvg      *float64 `json:"spam_rate_avg"`
	SpamRateMax      *float64 `json:"spam_rate_max"`
	DaysOverWarn     int      `json:"days_over_warn"`
	DaysOverCritical int      `json:"days_over_critical"`
	SPFAvg           *float64 `json:"spf_avg"`
	DKIMAvg          *float64 `json:"dkim_avg"`
	DMARCAvg         *float64 `json:"dmarc_avg"`
}

// PostmasterDomain is one sending domain of the workspace's mailboxes.
type PostmasterDomain struct {
	Domain     string  `json:"domain"`
	Mailboxes  int     `json:"mailboxes"`
	LatestDate *string `json:"latest_date"`
}

// PostmasterThresholds are the spam-rate lines the trend is drawn against.
type PostmasterThresholds struct {
	SpamWarn     float64 `json:"spam_warn"`
	SpamCritical float64 `json:"spam_critical"`
}

// PostmasterReport is the GET /deliverability/postmaster answer. Not
// connected, or no data yet, is a normal 200 with empty data so the dashboard
// can explain the setup instead of showing an error.
type PostmasterReport struct {
	Connected    bool                 `json:"connected"`
	CanConnect   bool                 `json:"can_connect"`
	LastSyncedAt *time.Time           `json:"last_synced_at"`
	LastError    string               `json:"last_error"`
	Domains      []PostmasterDomain   `json:"domains"`
	Domain       string               `json:"domain"`
	Days         int                  `json:"days"`
	Thresholds   PostmasterThresholds `json:"thresholds"`
	Summary      PostmasterSummary    `json:"summary"`
	Data         []PostmasterDay      `json:"data"`
}

// PostmasterCredentials is the Google OAuth client plus the refresh token of
// the account that owns the domains in Postmaster Tools. Sealed at rest.
type PostmasterCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// PostmasterConnection is the instance's stored credentials plus the last sync outcome.
type PostmasterConnection struct {
	Credentials PostmasterCredentials
	SyncedAt    *time.Time
	LastError   string
}

// PostmasterStat is one domain-day as pulled from Google, before it is stored.
type PostmasterStat struct {
	Date             time.Time
	DomainReputation string // high, medium, low, bad, or empty when Google gave none
	SpamRate         *float64
	SPFRatio         *float64
	DKIMRatio        *float64
	DMARCRatio       *float64
	Raw              json.RawMessage
}

// PostmasterOrgDomain pairs a workspace with one sending domain of its mailboxes.
type PostmasterOrgDomain struct {
	OrganizationID uuid.UUID
	Domain         string
}
