package models

import (
	"strings"
	"testing"
)

func TestBrandFooter(t *testing.T) {
	b := &Brand{Name: "Acme & Co", PhysicalAddress: "1 Main St\nSpringfield", FooterEnabled: true, UnsubscribeLink: true}
	plain, html := b.Footer("https://app.example.com/unsubscribe?cid=1&rid=2")
	if !strings.Contains(plain, "Acme & Co\n1 Main St\nSpringfield\nUnsubscribe: https://app.example.com/unsubscribe?cid=1&rid=2") {
		t.Errorf("plain footer = %q", plain)
	}
	if !strings.Contains(html, "Acme &amp; Co") || !strings.Contains(html, "1 Main St<br>Springfield") || !strings.Contains(html, `href="https://app.example.com/unsubscribe?cid=1&amp;rid=2"`) {
		t.Errorf("html footer = %q", html)
	}
	if p, _ := b.Footer(""); strings.Contains(p, "Unsubscribe") {
		t.Error("no unsubscribe line without a URL")
	}
	b.FooterEnabled = false
	if p, h := b.Footer("x"); p != "" || h != "" {
		t.Error("disabled footer rendered")
	}
	var nilBrand *Brand
	if p, _ := nilBrand.Footer("x"); p != "" {
		t.Error("nil brand rendered")
	}
}
