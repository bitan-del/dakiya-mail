package imap

import (
	"errors"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/emersion/go-imap/v2"
)

func (c *Client) handleError(err error) *errx.MailError {
	var imapErr *imap.Error
	if errors.As(err, &imapErr) {
		switch imapErr.Code {
		case imap.ResponseCodeAuthenticationFailed:
			if c.AuthType == models.AuthOAuth2 {
				return errx.ErrMailAuthenticationFailed
			} else {
				return errx.ErrMailInvalidCredentials
			}
		case imap.ResponseCodeAuthorizationFailed:
			return errx.ErrMailAuthorizationFailed
		default:
			return errx.ErrMailUnknownImapError(string(imapErr.Code))
		}
	}

	return nil
}
