package wmail

import (
	"context"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
)

func (w *WMail) SyncMail(ctx context.Context) *errx.MailError {
	switch w.EmailType {
	case models.InboxProviderGoogle:
		return w.SyncGoogle(ctx)
	case models.InboxProviderOutlook:
		return w.SyncGraph(ctx)
	case models.InboxProviderSMTPIMAP:
		return w.Sync(ctx)
	default:
		return nil
	}
}
