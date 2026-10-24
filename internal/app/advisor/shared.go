package advisor

import (
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

// Bands and floors the pre-send gate reads, so the gate and the Advisor never
// disagree about what a bad rate or an unsafe cap is.
const (
	ComplaintRateWarn        = complaintRateWarn
	ComplaintRateCritical    = complaintRateCritical
	BounceRateWarn           = bounceRateWarn
	BounceRateCritical       = bounceRateCritical
	MinSendsForComplaintRate = minSendsForComplaintRate
	MinSendsForBounceRate    = minSendsForBounceRate
	SafeColdCapBand          = safeColdCapBand
	NewMailboxDays           = newMailboxDays
	NewMailboxSafeCap        = newMailboxSafeCap
	RoleAddressShareWarn     = roleAddressShareWarn
)

// Rate is n as a percentage of total, 0 when there is no total.
func Rate(n, total int) float64 { return rate(n, total) }

// MissingAuthRecords lists the SPF, DKIM and DMARC records a mailbox's sending
// domain lacks. resolved is false until the sweep has checked the domain, and
// callers must not read that as failing.
func MissingAuthRecords(m repository.AdvisorMailbox) (missing []string, resolved bool) {
	if m.AuthState == models.AuthStateUnknown || m.AuthState == "" {
		return nil, false
	}
	if !m.AuthSPF {
		missing = append(missing, "SPF")
	}
	if !m.AuthDKIM {
		missing = append(missing, "DKIM")
	}
	if !m.AuthDMARC {
		missing = append(missing, "DMARC")
	}
	return missing, true
}
