package replyclassify

import "strings"

// classifyHeaders is Layer 1: a deterministic, offline scan of the message
// headers (plus subject) for the well-known machine-reply markers. It returns
// (result, true) when it definitively recognizes an automated message; (zero,
// false) otherwise so the pipeline falls through to the lexicon/model layers.
//
// We map out_of_office vs auto_reply where the signal distinguishes them, and
// fold bounces / delivery-status reports into auto_reply (a non-human, machine
// reply). Bounces are NOT given a distinct class on purpose: campaign branching
// only needs "automated vs human", a hard bounce already drives suppression and
// the dedicated bounced_at signal elsewhere, and a separate "bounce" class would
// have no branch field to route on. This is documented here as the deliberate
// choice for the contract's "bounces => ... your call but document it" clause.
func classifyHeaders(in Input) (Result, bool) {
	h := newHeaderLookup(in.Headers)
	subject := strings.ToLower(strings.TrimSpace(in.Subject))

	// --- Out-of-office signals (most specific machine reply) ---
	// Subject conventions providers emit for vacation autoresponders.
	if strings.HasPrefix(subject, "out of office") ||
		strings.HasPrefix(subject, "out of the office") ||
		strings.Contains(subject, "automatic reply") ||
		strings.Contains(subject, "auto-reply") ||
		strings.HasPrefix(subject, "autoreply") ||
		strings.HasPrefix(subject, "auto:") ||
		strings.Contains(subject, "on vacation") ||
		strings.Contains(subject, "on holiday") ||
		strings.Contains(subject, "away from") {
		return Result{Class: ClassOutOfOffice, Confidence: 0.98, Source: SourceHeader}, true
	}

	// RFC 3834 Auto-Submitted. "auto-replied" is canonically a vacation/auto
	// responder; "auto-generated" is any machine-generated message.
	if as := strings.ToLower(h.first("Auto-Submitted")); as != "" && as != "no" {
		if strings.Contains(as, "auto-replied") {
			return Result{Class: ClassOutOfOffice, Confidence: 0.95, Source: SourceHeader}, true
		}
		return Result{Class: ClassAutoReply, Confidence: 0.95, Source: SourceHeader}, true
	}

	// Vendor auto-responder headers (set by Exchange, Zimbra, helpdesks, etc.).
	if h.has("X-Autoreply") || h.has("X-Autorespond") ||
		strings.EqualFold(h.first("X-Autoreply"), "yes") {
		return Result{Class: ClassOutOfOffice, Confidence: 0.93, Source: SourceHeader}, true
	}
	if ars := h.first("X-Auto-Response-Suppress"); ars != "" {
		// Present on Exchange auto-responses; the message itself is automated.
		return Result{Class: ClassAutoReply, Confidence: 0.9, Source: SourceHeader}, true
	}

	// Precedence: bulk/junk/auto_reply marks list/automated traffic.
	if prec := strings.ToLower(h.first("Precedence")); prec != "" {
		switch prec {
		case "auto_reply":
			return Result{Class: ClassAutoReply, Confidence: 0.9, Source: SourceHeader}, true
		case "bulk", "junk", "list":
			return Result{Class: ClassAutoReply, Confidence: 0.75, Source: SourceHeader}, true
		}
	}

	// --- Bounce / delivery-status report signals => auto_reply (machine) ---
	// multipart/report; report-type=delivery-status is a DSN bounce.
	ct := strings.ToLower(h.first("Content-Type"))
	if strings.Contains(ct, "multipart/report") &&
		(strings.Contains(ct, "delivery-status") || strings.Contains(ct, "disposition-notification")) {
		return Result{Class: ClassAutoReply, Confidence: 0.95, Source: SourceHeader}, true
	}

	// Null Return-Path <> is the canonical bounce / non-reply-expecting envelope.
	if rp := strings.TrimSpace(h.first("Return-Path")); rp == "<>" || rp == "" && h.has("Return-Path") {
		return Result{Class: ClassAutoReply, Confidence: 0.85, Source: SourceHeader}, true
	}

	// mailer-daemon / postmaster style senders are machine bounce sources.
	if from := strings.ToLower(h.first("From")); from != "" {
		if strings.Contains(from, "mailer-daemon") ||
			strings.Contains(from, "postmaster@") ||
			strings.Contains(from, "no-reply@") ||
			strings.Contains(from, "noreply@") ||
			strings.Contains(from, "donotreply@") {
			return Result{Class: ClassAutoReply, Confidence: 0.8, Source: SourceHeader}, true
		}
	}

	return Result{}, false
}

// headerLookup is a case-insensitive view over an email header map. MIME header
// maps are normally canonical-cased ("Auto-Submitted"), but inbound sync may
// hand us lower-cased keys, so we index both.
type headerLookup struct {
	byLower map[string][]string
}

func newHeaderLookup(h map[string][]string) headerLookup {
	m := make(map[string][]string, len(h))
	for k, v := range h {
		m[strings.ToLower(k)] = v
	}
	return headerLookup{byLower: m}
}

func (h headerLookup) first(name string) string {
	if vs := h.byLower[strings.ToLower(name)]; len(vs) > 0 {
		return strings.TrimSpace(vs[0])
	}
	return ""
}

func (h headerLookup) has(name string) bool {
	_, ok := h.byLower[strings.ToLower(name)]
	return ok
}
