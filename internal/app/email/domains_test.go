package email

import (
	"testing"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

func TestSPFValue(t *testing.T) {
	cases := []struct {
		existing, include, want string
		ok                      bool
	}{
		{"", "include:amazonses.com", "v=spf1 include:amazonses.com ~all", false},
		{"v=spf1 include:_spf.google.com ~all", "include:amazonses.com", "v=spf1 include:_spf.google.com include:amazonses.com ~all", false},
		{"v=spf1 include:amazonses.com -all", "include:amazonses.com", "v=spf1 include:amazonses.com -all", true},
		{"v=spf1 mx", "include:amazonses.com", "v=spf1 mx include:amazonses.com", false},
	}
	for _, c := range cases {
		got, ok := spfValue(c.existing, c.include)
		if got != c.want || ok != c.ok {
			t.Errorf("spfValue(%q, %q) = %q, %v; want %q, %v", c.existing, c.include, got, ok, c.want, c.ok)
		}
	}
}

func TestBuildDNSRecordsSES(t *testing.T) {
	now := time.Now()
	d := &models.SendingDomain{
		Domain: "mail.example.com", Provider: models.SendingProviderSES,
		DKIMSelectors: []string{"abc123", "def456", "ghi789"},
		SPFFound:      true, SPFRecord: "v=spf1 include:amazonses.com ~all",
		DKIMFound: true, LastCheckedAt: &now,
	}
	recs := BuildDNSRecords(d)
	if len(recs) != 5 {
		t.Fatalf("got %d records, want spf + 3 dkim + dmarc", len(recs))
	}
	if recs[0].Found == nil || !*recs[0].Found {
		t.Error("SPF with the SES include should read as found")
	}
	if recs[1].Host != "abc123._domainkey.mail.example.com" || recs[1].Value != "abc123.dkim.amazonses.com" || recs[1].Type != "CNAME" {
		t.Errorf("unexpected SES DKIM record %+v", recs[1])
	}
	if recs[4].Found == nil || *recs[4].Found {
		t.Error("missing DMARC should read as not found")
	}
}

func TestNormalizeDomainRequest(t *testing.T) {
	req := &models.SaveSendingDomainRequest{Domain: " Sales@Mail.Example.COM. ", DKIMSelectors: []string{"S1._domainkey", "s1", ""}}
	if xerr := normalizeDomainRequest(req); xerr != nil {
		t.Fatal(xerr)
	}
	if req.Domain != "mail.example.com" || req.Provider != models.SendingProviderOther || len(req.DKIMSelectors) != 1 || req.DKIMSelectors[0] != "s1" {
		t.Errorf("normalized to %+v", req)
	}
	if normalizeDomainRequest(&models.SaveSendingDomainRequest{Domain: "not a domain"}) == nil {
		t.Error("invalid domain accepted")
	}
}
