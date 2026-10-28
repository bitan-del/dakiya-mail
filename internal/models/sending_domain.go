package models

import (
	"time"

	"github.com/google/uuid"
)

type SendingDomainProvider string

const (
	SendingProviderGoogle    SendingDomainProvider = "google_workspace"
	SendingProviderMicrosoft SendingDomainProvider = "microsoft_365"
	SendingProviderSES       SendingDomainProvider = "amazon_ses"
	SendingProviderOther     SendingDomainProvider = "other"
)

func (p SendingDomainProvider) Valid() bool {
	switch p {
	case SendingProviderGoogle, SendingProviderMicrosoft, SendingProviderSES, SendingProviderOther:
		return true
	}
	return false
}

type SendingDomain struct {
	ID             uuid.UUID             `json:"id"`
	OrganizationID uuid.UUID             `json:"organization_id"`
	Domain         string                `json:"domain"`
	Provider       SendingDomainProvider `json:"provider"`
	DKIMSelectors  []string              `json:"dkim_selectors"`
	State          string                `json:"state"`
	SPFFound       bool                  `json:"spf_found"`
	SPFRecord      string                `json:"spf_record"`
	DKIMFound      bool                  `json:"dkim_found"`
	DMARCFound     bool                  `json:"dmarc_found"`
	DMARCPolicy    string                `json:"dmarc_policy"`
	CheckSummary   string                `json:"check_summary"`
	LastCheckedAt  *time.Time            `json:"last_checked_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`

	// Derived on read.
	MailboxCount int         `json:"mailbox_count"`
	Records      []DNSRecord `json:"records"`
}

// DNSRecord is one record the domain should publish, with whether DNS has it.
type DNSRecord struct {
	Purpose string `json:"purpose"` // spf, dkim, dmarc
	Type    string `json:"type"`    // TXT or CNAME
	Host    string `json:"host"`
	Value   string `json:"value"`
	// Found is the last check's verdict; nil when the record cannot be checked
	// (a DKIM key only the provider's console can show).
	Found *bool  `json:"found"`
	Note  string `json:"note,omitempty"`
}

type SaveSendingDomainRequest struct {
	Domain        string                `json:"domain"`
	Provider      SendingDomainProvider `json:"provider"`
	DKIMSelectors []string              `json:"dkim_selectors"`
}
