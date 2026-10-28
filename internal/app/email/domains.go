package email

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/dnsauth"
	"github.com/google/uuid"
)

var (
	domainPattern   = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	selectorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
)

// spfInclude is the include each provider's SPF needs; "" means the user's own.
var spfInclude = map[models.SendingDomainProvider]string{
	models.SendingProviderGoogle:    "include:_spf.google.com",
	models.SendingProviderMicrosoft: "include:spf.protection.outlook.com",
	models.SendingProviderSES:       "include:amazonses.com",
}

func normalizeDomainRequest(req *models.SaveSendingDomainRequest) *errx.Error {
	d := strings.ToLower(strings.TrimSpace(req.Domain))
	if at := strings.LastIndex(d, "@"); at >= 0 {
		d = d[at+1:]
	}
	d = strings.TrimSuffix(d, ".")
	if !domainPattern.MatchString(d) {
		return errx.NewWithIdentifier(errx.BadRequest, "invalid_domain", "enter a domain such as mail.example.com")
	}
	req.Domain = d
	if req.Provider == "" {
		req.Provider = models.SendingProviderOther
	}
	if !req.Provider.Valid() {
		return errx.New(errx.BadRequest, "provider must be google_workspace, microsoft_365, amazon_ses or other")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range req.DKIMSelectors {
		s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "._domainkey")))
		if s == "" || seen[s] {
			continue
		}
		if !selectorPattern.MatchString(s) || len(out) >= 10 {
			return errx.NewWithIdentifier(errx.BadRequest, "invalid_dkim_selector", "DKIM selectors are letters, digits, dots and dashes, at most 10")
		}
		seen[s] = true
		out = append(out, s)
	}
	req.DKIMSelectors = out
	return nil
}

// probeSelectors are the DKIM selectors the check looks up for the provider.
func probeSelectors(d *models.SendingDomain) []string {
	if len(d.DKIMSelectors) > 0 {
		return d.DKIMSelectors
	}
	switch d.Provider {
	case models.SendingProviderGoogle:
		return []string{"google"}
	case models.SendingProviderMicrosoft:
		return []string{"selector1", "selector2"}
	}
	return nil
}

// spfValue is the SPF record to publish: the existing one with the provider's
// include added, or a fresh one.
func spfValue(existing, include string) (string, bool) {
	if include == "" {
		if existing != "" {
			return existing, true
		}
		return "v=spf1 include:<your provider> ~all", false
	}
	if existing == "" {
		return "v=spf1 " + include + " ~all", false
	}
	if strings.Contains(strings.ToLower(existing), strings.ToLower(include)) {
		return existing, true
	}
	fields := strings.Fields(existing)
	last := fields[len(fields)-1]
	if strings.HasSuffix(last, "all") {
		return strings.Join(append(append(fields[:len(fields)-1:len(fields)-1], include), last), " "), false
	}
	return existing + " " + include, false
}

// BuildDNSRecords lists what the domain should publish for its provider, marked
// against the last check.
func BuildDNSRecords(d *models.SendingDomain) []models.DNSRecord {
	checked := d.LastCheckedAt != nil
	found := func(ok bool) *bool {
		if !checked {
			return nil
		}
		return &ok
	}

	spf, spfOK := spfValue(d.SPFRecord, spfInclude[d.Provider])
	spfNote := ""
	if d.SPFRecord != "" && !spfOK {
		spfNote = "Replace your existing SPF record with this one; a domain may only have one."
	}
	records := []models.DNSRecord{{
		Purpose: "spf", Type: "TXT", Host: d.Domain, Value: spf,
		Found: found(d.SPFFound && spfOK), Note: spfNote,
	}}

	dkimFound := found(d.DKIMFound)
	switch d.Provider {
	case models.SendingProviderGoogle:
		records = append(records, models.DNSRecord{
			Purpose: "dkim", Type: "TXT", Host: "google._domainkey." + d.Domain,
			Value: "The key from Google Admin: Apps, Google Workspace, Gmail, Authenticate email",
			Found: dkimFound, Note: "Generate the key in Google Admin, publish it, then press Start authentication there.",
		})
	case models.SendingProviderMicrosoft:
		dashed := strings.ReplaceAll(d.Domain, ".", "-")
		for _, sel := range []string{"selector1", "selector2"} {
			records = append(records, models.DNSRecord{
				Purpose: "dkim", Type: "CNAME", Host: sel + "._domainkey." + d.Domain,
				Value: sel + "-" + dashed + "._domainkey.<your tenant>.onmicrosoft.com",
				Found: dkimFound, Note: "Microsoft Defender, Email authentication, DKIM shows the exact value for your tenant.",
			})
		}
	case models.SendingProviderSES:
		if len(d.DKIMSelectors) == 0 {
			records = append(records, models.DNSRecord{
				Purpose: "dkim", Type: "CNAME", Host: "<token>._domainkey." + d.Domain, Value: "<token>.dkim.amazonses.com",
				Note: "Add the three Easy DKIM tokens from the SES console (Identities, your domain, DKIM) to get the exact records.",
			})
		}
		for _, tok := range d.DKIMSelectors {
			records = append(records, models.DNSRecord{
				Purpose: "dkim", Type: "CNAME", Host: tok + "._domainkey." + d.Domain, Value: tok + ".dkim.amazonses.com", Found: dkimFound,
			})
		}
	default:
		if len(d.DKIMSelectors) == 0 {
			records = append(records, models.DNSRecord{
				Purpose: "dkim", Type: "TXT", Host: "<selector>._domainkey." + d.Domain, Value: "Your provider's DKIM public key",
				Note: "Add your provider's DKIM selector so it can be checked.",
			})
		}
		for _, sel := range d.DKIMSelectors {
			records = append(records, models.DNSRecord{
				Purpose: "dkim", Type: "TXT", Host: sel + "._domainkey." + d.Domain, Value: "Your provider's DKIM public key", Found: dkimFound,
			})
		}
	}

	dmarcNote := "Start at p=none to collect reports, then move to p=quarantine once they show only your own mail."
	dmarcValue := "v=DMARC1; p=none; rua=mailto:dmarc@" + d.Domain
	if d.DMARCFound {
		dmarcValue = "v=DMARC1; p=" + d.DMARCPolicy
		dmarcNote = "A DMARC policy is published (it may be inherited from the parent domain)."
	}
	records = append(records, models.DNSRecord{
		Purpose: "dmarc", Type: "TXT", Host: "_dmarc." + d.Domain, Value: dmarcValue, Found: found(d.DMARCFound), Note: dmarcNote,
	})
	return records
}

func withRecords(d *models.SendingDomain) *models.SendingDomain {
	d.Records = BuildDNSRecords(d)
	return d
}

func (s *emailService) ListSendingDomains(ctx context.Context, orgID uuid.UUID) ([]models.SendingDomain, *errx.Error) {
	list, xerr := s.emailRepository.ListSendingDomains(ctx, orgID)
	if xerr != nil {
		return nil, xerr
	}
	for i := range list {
		withRecords(&list[i])
	}
	return list, nil
}

func (s *emailService) GetSendingDomain(ctx context.Context, orgID, id uuid.UUID) (*models.SendingDomain, *errx.Error) {
	d, xerr := s.emailRepository.GetSendingDomain(ctx, orgID, id)
	if xerr != nil {
		return nil, xerr
	}
	return withRecords(d), nil
}

// AddSendingDomain saves the domain and checks it straight away.
func (s *emailService) AddSendingDomain(ctx context.Context, orgID uuid.UUID, req *models.SaveSendingDomainRequest) (*models.SendingDomain, *errx.Error) {
	if xerr := normalizeDomainRequest(req); xerr != nil {
		return nil, xerr
	}
	id, xerr := s.emailRepository.CreateSendingDomain(ctx, orgID, req)
	if xerr != nil {
		return nil, xerr
	}
	return s.VerifySendingDomain(ctx, orgID, id)
}

func (s *emailService) UpdateSendingDomain(ctx context.Context, orgID, id uuid.UUID, req *models.SaveSendingDomainRequest) (*models.SendingDomain, *errx.Error) {
	current, xerr := s.emailRepository.GetSendingDomain(ctx, orgID, id)
	if xerr != nil {
		return nil, xerr
	}
	req.Domain = current.Domain
	if xerr := normalizeDomainRequest(req); xerr != nil {
		return nil, xerr
	}
	if xerr := s.emailRepository.UpdateSendingDomainSetup(ctx, orgID, id, req.Provider, req.DKIMSelectors); xerr != nil {
		return nil, xerr
	}
	return s.VerifySendingDomain(ctx, orgID, id)
}

func (s *emailService) DeleteSendingDomain(ctx context.Context, orgID, id uuid.UUID) *errx.Error {
	return s.emailRepository.DeleteSendingDomain(ctx, orgID, id)
}

// VerifySendingDomain runs the DNS check, records it, and refreshes the
// authentication state of every mailbox on the domain so a fixed domain
// clears the send gate at once.
func (s *emailService) VerifySendingDomain(ctx context.Context, orgID, id uuid.UUID) (*models.SendingDomain, *errx.Error) {
	d, xerr := s.emailRepository.GetSendingDomain(ctx, orgID, id)
	if xerr != nil {
		return nil, xerr
	}
	res := dnsauth.Check(ctx, d.Domain, probeSelectors(d))
	now := time.Now().UTC()
	if xerr := s.emailRepository.RecordSendingDomainCheck(ctx, d.ID, res.State(), res.SPFFound, res.SPFRecord, res.DKIMFound, res.DMARCFound, res.DMARCPolicy, res.Summary, now); xerr != nil {
		return nil, xerr
	}
	if d.MailboxCount > 0 {
		_, _ = s.emailRepository.UpdateDomainAuthState(ctx, d.Domain, res.State(), res.SPFFound, res.DKIMFound, res.DMARCFound, res.DMARCPolicy, res.Summary, now)
	}
	return s.GetSendingDomain(ctx, orgID, id)
}
