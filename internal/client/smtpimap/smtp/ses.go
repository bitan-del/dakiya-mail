package smtp

import (
	"context"
	"errors"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/client/sesapi"
	"github.com/bitan-del/dakiya-mail/internal/errx"
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

// sendViaSESAPI returns "Ok <id>", the same shape SES SMTP replies with, so the
// worker reads the message id the same way for both.
func (c *Client) sendViaSESAPI(ctx context.Context, region, from string, to []string, data []byte) (string, *errx.MailError) {
	id, err := sesapi.Send(ctx, region, c.Credentials.Username, c.Credentials.Password, c.BindIP, from, to, data)
	if err != nil {
		var se *sesapi.SendError
		if errors.As(err, &se) {
			switch se.Kind {
			case sesapi.FailureCredentials:
				return "", errx.ErrMailInvalidCredentials
			case sesapi.FailureSender:
				return "", errx.ErrMailDomainAuthRejected
			case sesapi.FailureRecipient:
				return "", errx.ErrMailRecipientRejected
			}
		}
		return "", errx.ErrMailServerUnreachable
	}
	return "Ok " + id, nil
}
