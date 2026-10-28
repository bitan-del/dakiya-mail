package models

import (
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Brand struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	Name            string    `json:"name"`
	Website         string    `json:"website"`
	PhysicalAddress string    `json:"physical_address"`
	FooterText      string    `json:"footer_text"`
	FooterEnabled   bool      `json:"footer_enabled"`
	UnsubscribeLink bool      `json:"unsubscribe_link"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// MailboxIDs are the mailboxes sending as this brand (list and get only).
	MailboxIDs []uuid.UUID `json:"mailbox_ids"`
}

type SaveBrandRequest struct {
	Name            string `json:"name"`
	Website         string `json:"website"`
	PhysicalAddress string `json:"physical_address"`
	FooterText      string `json:"footer_text"`
	FooterEnabled   *bool  `json:"footer_enabled"`
	UnsubscribeLink *bool  `json:"unsubscribe_link"`
}

type SetBrandMailboxesRequest struct {
	EmailAccountIDs []string `json:"email_account_ids"`
}

// Footer renders the brand footer appended to campaign mail. unsubscribeURL may
// be empty; it is only linked when the brand asks for it.
func (b *Brand) Footer(unsubscribeURL string) (plain, htmlPart string) {
	if b == nil || !b.FooterEnabled {
		return "", ""
	}
	var lines []string
	for _, l := range []string{b.Name, b.PhysicalAddress, b.FooterText} {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	link := b.UnsubscribeLink && unsubscribeURL != ""
	if len(lines) == 0 && !link {
		return "", ""
	}
	plainLines := append([]string{}, lines...)
	htmlLines := make([]string, 0, len(lines)+1)
	for _, l := range lines {
		htmlLines = append(htmlLines, strings.ReplaceAll(html.EscapeString(l), "\n", "<br>"))
	}
	if link {
		plainLines = append(plainLines, "Unsubscribe: "+unsubscribeURL)
		htmlLines = append(htmlLines, `<a href="`+html.EscapeString(unsubscribeURL)+`" style="color:#888888">Unsubscribe</a>`)
	}
	plain = "\n\n--\n" + strings.Join(plainLines, "\n")
	htmlPart = `<div style="margin-top:24px;padding-top:12px;border-top:1px solid #e5e5e5;font-size:12px;line-height:1.5;color:#888888">` +
		strings.Join(htmlLines, "<br>") + `</div>`
	return plain, htmlPart
}
