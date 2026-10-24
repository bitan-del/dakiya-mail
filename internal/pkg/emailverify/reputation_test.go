package emailverify

import (
	"net"
	"testing"
)

func TestIsDisposable(t *testing.T) {
	cases := []struct {
		domain string
		want   bool
	}{
		{"mailinator.com", true},
		{"MAILINATOR.COM", true},        // case-insensitive
		{"mailinator.com.", true},       // trailing root dot
		{"user.trashmail.com", true},    // per-user subdomain
		{"a.b.guerrillamail.com", true}, // deeper subdomain
		{"gmail.com", false},
		{"yourdomain.com", false},
		{"com", false}, // bare TLD must never match
		{"", false},
	}
	for _, c := range cases {
		if got := IsDisposable(c.domain); got != c.want {
			t.Errorf("IsDisposable(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}

func TestIsRoleAccount(t *testing.T) {
	cases := []struct {
		localpart string
		want      bool
	}{
		{"info", true},
		{"INFO", true},
		{"support", true},
		{"no-reply", true},
		{"no.reply", true},  // separator variant
		{"noreply", true},   // canonical
		{"sales+eu", true},  // plus-addressing stripped
		{"help_desk", true}, // underscore variant of helpdesk
		{"ana", false},
		{"ana.ruiz", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsRoleAccount(c.localpart); got != c.want {
			t.Errorf("IsRoleAccount(%q) = %v, want %v", c.localpart, got, c.want)
		}
	}
}

// A disposable address must be rejected on shape alone, without any network
// call — the domain has MX and accepts mail, so waiting for the probe would
// report it valid.
func TestVerifyRejectsDisposableWithoutProbing(t *testing.T) {
	v := New(Config{HeloHost: "mail.example.com"})
	res := v.Verify(t.Context(), "someone@mailinator.com")
	if res.Status != StatusInvalid {
		t.Fatalf("status = %q, want %q", res.Status, StatusInvalid)
	}
	if !res.IsDisposable {
		t.Error("IsDisposable should be set")
	}
	if res.HasMX {
		t.Error("should have short-circuited before the MX lookup")
	}
}

// RFC 7505 §3: a domain that accepts no mail publishes a single MX with a root
// target. It must be read as an explicit refusal, and must NOT fall through to
// the implicit-MX A-record fallback — example.com publishes exactly this and
// also has an A record, so the fallback would happily probe it.
func TestIsNullMX(t *testing.T) {
	cases := []struct {
		name string
		mxs  []*net.MX
		want bool
	}{
		{"root target", []*net.MX{{Host: ".", Pref: 0}}, true},
		{"empty target", []*net.MX{{Host: "", Pref: 0}}, true},
		{"padded root", []*net.MX{{Host: " . ", Pref: 0}}, true},
		{"normal single MX", []*net.MX{{Host: "mx.example.net.", Pref: 10}}, false},
		{"no records", nil, false},
		// More than one record is a normal mail domain, however odd; the null
		// MX form is defined as the single-record case only.
		{"root plus a real host", []*net.MX{{Host: ".", Pref: 0}, {Host: "mx.example.net.", Pref: 10}}, false},
	}
	for _, c := range cases {
		if got := isNullMX(c.mxs); got != c.want {
			t.Errorf("isNullMX(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
