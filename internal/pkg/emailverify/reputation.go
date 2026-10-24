package emailverify

import "strings"

// Address-shape signals that matter for cold outreach but that an SMTP probe
// cannot tell you, because both kinds of address are usually perfectly
// deliverable — they are just bad to mail.
//
// These are independent of the network checks and run before them: they cost
// nothing, and a disposable address is worth flagging even when the probe is
// skipped or inconclusive.
//
// Written from the published definitions of these categories, not ported from
// any existing verifier — deliberately, since the well-known implementations
// are AGPL and this file ships under the project's Apache licence.

// roleLocalparts are shared/functional mailboxes rather than a person. They are
// usually real and accepting, so an SMTP probe calls them valid — but they are
// read by several people or none, they attract spam complaints, and several
// blocklist operators seed them. Mailing them in cold outreach is a
// reputational liability, so they are surfaced as risky rather than valid.
var roleLocalparts = map[string]struct{}{
	"abuse": {}, "admin": {}, "administrator": {}, "billing": {}, "compliance": {},
	"contact": {}, "customerservice": {}, "enquiries": {}, "enquiry": {}, "feedback": {},
	"finance": {}, "help": {}, "helpdesk": {}, "hello": {}, "hi": {}, "hostmaster": {},
	"info": {}, "inquiries": {}, "inquiry": {}, "it": {}, "jobs": {}, "legal": {},
	"mail": {}, "mailer-daemon": {}, "marketing": {}, "media": {}, "newsletter": {},
	"noc": {}, "no-reply": {}, "noreply": {}, "office": {}, "orders": {}, "postmaster": {},
	"press": {}, "privacy": {}, "recruitment": {}, "root": {}, "sales": {}, "security": {},
	"service": {}, "spam": {}, "support": {}, "sysadmin": {}, "team": {}, "webmaster": {},
	"welcome": {},
}

// disposableDomains are throwaway/temporary-inbox providers. Mail to them is
// accepted and then discarded, so an SMTP probe reports a healthy valid address
// for a mailbox that no person will ever read.
//
// This is a starter set of the largest operators, not an exhaustive list —
// there are thousands of these domains and they churn constantly. Point
// DisposableDomains at a maintained list (several are published under
// permissive licences) to widen it without touching this file.
var disposableDomains = map[string]struct{}{
	"0-mail.com": {}, "10minutemail.com": {}, "20minutemail.com": {}, "33mail.com": {},
	"anonbox.net": {}, "bugmenot.com": {}, "burnermail.io": {}, "byom.de": {},
	"dispostable.com": {}, "e4ward.com": {}, "emailondeck.com": {}, "fakeinbox.com": {},
	"fakemailgenerator.com": {}, "getairmail.com": {}, "getnada.com": {}, "grr.la": {},
	"guerrillamail.biz": {}, "guerrillamail.com": {}, "guerrillamail.de": {},
	"guerrillamail.net": {}, "guerrillamail.org": {}, "guerrillamailblock.com": {},
	"harakirimail.com": {}, "inboxbear.com": {}, "incognitomail.com": {}, "jetable.org": {},
	"mail-temporaire.fr": {}, "mail7.io": {}, "mailcatch.com": {}, "maildrop.cc": {},
	"mailinator.com": {}, "mailnesia.com": {}, "mailsac.com": {}, "mintemail.com": {},
	"mohmal.com": {}, "moakt.com": {}, "mytemp.email": {}, "nada.email": {},
	"sharklasers.com": {}, "spam4.me": {}, "spamgourmet.com": {}, "temp-mail.io": {},
	"temp-mail.org": {}, "tempail.com": {}, "tempinbox.com": {}, "tempmail.net": {},
	"tempmailo.com": {}, "throwawaymail.com": {}, "trashmail.com": {}, "trashmail.de": {},
	"trbvm.com": {}, "yopmail.com": {}, "yopmail.fr": {}, "yopmail.net": {},
}

// DisposableDomains lets a deployment swap in a maintained list. Nil keeps the
// built-in starter set. Domains must be lowercase and bare (no leading dot).
var DisposableDomains map[string]struct{}

// IsDisposable reports whether the domain belongs to a throwaway-inbox
// provider. A subdomain of a disposable domain counts: several of these
// operators hand out per-user subdomains.
func IsDisposable(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
	if domain == "" {
		return false
	}
	list := DisposableDomains
	if list == nil {
		list = disposableDomains
	}
	if _, ok := list[domain]; ok {
		return true
	}
	// Walk up the labels so mailbox.trashmail.com matches trashmail.com. Stop
	// before the bare TLD; a one-label suffix can never be in the list.
	for i := strings.Index(domain, "."); i >= 0; {
		domain = domain[i+1:]
		if strings.Count(domain, ".") == 0 {
			return false
		}
		if _, ok := list[domain]; ok {
			return true
		}
		i = strings.Index(domain, ".")
	}
	return false
}

// IsRoleAccount reports whether the localpart is a shared/functional mailbox.
// Plus-addressing and dots are normalized away first so "sales+eu@" and
// "no.reply@" are still caught.
func IsRoleAccount(localpart string) bool {
	lp := strings.ToLower(strings.TrimSpace(localpart))
	if i := strings.Index(lp, "+"); i >= 0 {
		lp = lp[:i]
	}
	if _, ok := roleLocalparts[lp]; ok {
		return true
	}
	// "no.reply" / "help.desk" — strip separators and retry, so a punctuation
	// variant of a known role name does not slip through.
	stripped := strings.NewReplacer(".", "", "-", "", "_", "").Replace(lp)
	_, ok := roleLocalparts[stripped]
	return ok
}
