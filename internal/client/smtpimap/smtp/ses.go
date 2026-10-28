package smtp

import (
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

// Host is the SMTP server this client submits to.
func (c *Client) Host() string {
	switch c.AuthType {
	case models.AuthPlain:
		if c.Credentials != nil {
			return c.Credentials.Host
		}
	case models.AuthOAuth2:
		if c.Oauth2 != nil {
			return c.Oauth2.Host
		}
	}
	return ""
}

// SESMessageID extracts the id Amazon SES returns in its "250 Ok <id>" reply to
// DATA. It is empty for any other server, whose replies carry no stable id.
func SESMessageID(host, reply string) string {
	if !strings.HasSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".amazonaws.com") {
		return ""
	}
	fields := strings.Fields(reply)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "ok") {
		return ""
	}
	return strings.Trim(fields[1], "<>")
}
