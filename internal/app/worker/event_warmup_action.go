package worker

import (
	"context"
	"slices"

	"github.com/bitan-del/dakiya-mail/internal/app/worker/wmail"
	"github.com/bitan-del/dakiya-mail/internal/client/smtpimap/imap"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/rs/zerolog/log"
)

// HandleWarmupAction executes recipient-side warmup actions on the mailbox.
//
// The worker just runs whatever actions it receives, immediately. The
// recipient-side "dwell" and the immediate-vs-delayed split are now owned by the
// CONSUMER's durable schedule (internal/app/consumer/warmup_engagement_poller):
// the consumer publishes the immediate leg (folder + spam-rescue) right away and
// the delayed leg (read / important / star) when its fire_at passes, each with
// DelaySeconds=0. That makes the dwell survive a worker restart, which the old
// in-process time.AfterFunc here could not.
func (w *WorkerService) HandleWarmupAction(ctx context.Context, action models.WarmupEmailAction) error {
	log.Info().
		Str("email_id", action.EmailID.String()).
		Str("gmail_id", action.GmailID).
		Uint32("uid", action.UID).
		Uint32("mailbox_uid_validity", action.MailboxUIDValidity).
		Strs("actions", action.Actions).
		Msg("Processing warmup email action")

	if len(action.Actions) == 0 {
		return nil
	}

	w.mailManager.RLock()
	mail, exists := w.mailManager.Emails[action.EmailID]
	w.mailManager.RUnlock()
	if !exists {
		log.Warn().Str("email_id", action.EmailID.String()).Msg("Email account not found for warmup action")
		return nil
	}

	switch {
	case mail.GoogleData != nil && mail.GoogleData.Client != nil:
		w.runGoogleWarmupActions(ctx, mail, action)
	case mail.GraphData != nil && mail.GraphData.Client != nil:
		w.runGraphWarmupActions(ctx, mail, action)
	case mail.SmtpImapData != nil && mail.SmtpImapData.ImapClient != nil:
		w.runImapWarmupActions(ctx, mail, action)
	default:
		log.Warn().
			Str("email_id", action.EmailID.String()).
			Msg("No mail client available for warmup actions; skipping")
	}
	return nil
}

func (w *WorkerService) runGoogleWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	for _, act := range action.Actions {
		switch act {
		case "move_to_dakiya":
			if err := mail.GoogleData.Client.ApplyLabel(ctx, action.GmailID, imap.WarmupFolderName); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to apply Dakiya Mail label")
			}
		case "mark_read":
			if err := mail.GoogleData.Client.MarkAsRead(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to mark as read")
			}
		case "remove_from_spam":
			if err := mail.GoogleData.Client.RemoveFromSpam(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to remove from spam")
			}
		case "mark_important":
			if err := mail.GoogleData.Client.MarkImportant(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to mark important")
			}
		case "star":
			if err := mail.GoogleData.Client.AddStar(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to star warmup message")
			}
		default:
			log.Warn().Str("action", act).Msg("Unknown warmup action")
		}
	}
}

// runGraphWarmupActions applies recipient-side warmup engagement to a Microsoft
// Graph mailbox. action.GmailID carries the Graph message id (the provider id
// field is provider-agnostic). Star maps to the follow-up flag, the closest
// Outlook equivalent of a Gmail star.
func (w *WorkerService) runGraphWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	client := mail.GraphData.Client

	// A Graph message id changes whenever the message is moved (copy+delete), so
	// resolve the live id from the immutable RFC Message-ID before acting. This
	// leg may run after an earlier leg already moved the message to Dakiya Mail.
	msgID := action.GmailID
	if action.RFCMessageID != "" {
		if resolved, err := client.ResolveMessageID(ctx, action.RFCMessageID); err == nil && resolved != "" {
			msgID = resolved
		}
	}

	for _, act := range action.Actions {
		switch act {
		case "move_to_dakiya":
			newID, err := client.MoveToFolder(ctx, msgID, imap.WarmupFolderName)
			if err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to move to Dakiya Mail folder (Graph)")
				continue
			}
			if newID != "" {
				msgID = newID // subsequent actions target the moved copy
			}
		case "mark_read":
			if err := client.MarkAsRead(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to mark as read (Graph)")
			}
		case "remove_from_spam":
			newID, err := client.RemoveFromSpam(ctx, msgID)
			if err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to rescue from junk (Graph)")
				continue
			}
			if newID != "" {
				msgID = newID
			}
		case "mark_important":
			if err := client.MarkImportant(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to mark important (Graph)")
			}
		case "star":
			if err := client.AddFlag(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to flag warmup message (Graph)")
			}
		default:
			log.Warn().Str("action", act).Msg("Unknown warmup action")
		}
	}
}

func (w *WorkerService) runImapWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	sourceBox := lookupMailboxByUIDValidity(mail.SmtpImapData.Mailboxes, action.MailboxUIDValidity)
	if sourceBox == nil {
		log.Warn().
			Uint32("uid_validity", action.MailboxUIDValidity).
			Str("email_id", action.EmailID.String()).
			Msg("Source mailbox for warmup action not found; skipping")
		return
	}

	inboxName := "INBOX"
	if inboxBox := lookupInbox(mail.SmtpImapData.Mailboxes); inboxBox != nil {
		inboxName = inboxBox.Name
	}

	steps, unknown := planImapWarmup(action.Actions, imap.IsSpamMailbox(sourceBox.Name, sourceBox.Attrs), action.RFCMessageID != "")
	for _, act := range unknown {
		log.Warn().Str("action", act).Msg("Unknown warmup action")
	}
	applyImapWarmup(ctx, mail.SmtpImapData.ImapClient, steps, sourceBox.Name, inboxName, action.UID, action.RFCMessageID)
}

// planImapWarmup orders actions flags, rescue, move so none targets a UID a move already retired; rescue then move needs the Message-ID, else the rescue wins.
func planImapWarmup(actions []string, sourceIsSpam, canResolve bool) (steps, unknown []string) {
	var read, important, rescue, file bool
	for _, a := range actions {
		switch a {
		case "mark_read":
			read = true
		case "mark_important":
			important = true
		case "remove_from_spam":
			rescue = rescue || sourceIsSpam
		case "move_to_dakiya":
			file = true
		case "star":
			// No-op on IMAP: \Flagged is already set by mark_important.
		default:
			unknown = append(unknown, a)
		}
	}
	if rescue && file && !canResolve {
		file = false
	}
	for _, step := range []struct {
		name string
		on   bool
	}{{"mark_read", read}, {"mark_important", important}, {"remove_from_spam", rescue}, {"move_to_dakiya", file}} {
		if step.on {
			steps = append(steps, step.name)
		}
	}
	return steps, unknown
}

// imapLookupOrder lists where a warmup message can live after our own moves: Dakiya Mail folder, arrival folder, INBOX.
func imapLookupOrder(sourceName, inboxName string) []string {
	order := []string{imap.WarmupFolderName}
	for _, name := range []string{sourceName, inboxName} {
		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}
	return order
}

// imapWarmupOps is the IMAP client surface warmup actions drive (wmail.ImapConn satisfies it).
type imapWarmupOps interface {
	MarkAsRead(ctx context.Context, mailboxName string, uid uint32) error
	MarkImportant(ctx context.Context, mailboxName string, uid uint32) error
	MoveToFolder(ctx context.Context, sourceMailbox, dstFolder string, uid uint32) error
	RemoveFromSpam(ctx context.Context, sourceMailbox, inboxName string, uid uint32) error
	FindByMessageID(ctx context.Context, rfcMessageID string, mailboxes []string) (mailbox string, uid uint32, err error)
}

// applyImapWarmup runs planned steps; a leg with no move is the delayed one, so it re-finds the message by Message-ID first.
func applyImapWarmup(ctx context.Context, ops imapWarmupOps, steps []string, sourceName, inboxName string, uid uint32, rfcMessageID string) {
	mailbox := sourceName // uid 0 means in mailbox, UID unknown
	find := func(in ...string) bool {
		if rfcMessageID == "" {
			return false
		}
		found, foundUID, err := ops.FindByMessageID(ctx, rfcMessageID, in)
		if err != nil {
			log.Error().Err(err).Str("message_id", rfcMessageID).Msg("Failed to locate warmup message (IMAP)")
			return false
		}
		if foundUID == 0 {
			log.Debug().Str("message_id", rfcMessageID).Msg("Warmup message no longer in any known folder; skipping")
			return false
		}
		mailbox, uid = found, foundUID
		return true
	}

	if len(steps) > 0 && rfcMessageID != "" && !slices.Contains(steps, "move_to_dakiya") && !slices.Contains(steps, "remove_from_spam") {
		if !find(imapLookupOrder(sourceName, inboxName)...) {
			return
		}
	}

	for _, step := range steps {
		var err error
		switch step {
		case "mark_read":
			err = ops.MarkAsRead(ctx, mailbox, uid)
		case "mark_important":
			err = ops.MarkImportant(ctx, mailbox, uid)
		case "remove_from_spam":
			if err = ops.RemoveFromSpam(ctx, mailbox, inboxName, uid); err == nil {
				mailbox, uid = inboxName, 0
			}
		case "move_to_dakiya":
			if uid == 0 && !find(mailbox) {
				return
			}
			err = ops.MoveToFolder(ctx, mailbox, imap.WarmupFolderName, uid)
		}
		if err != nil {
			log.Error().Err(err).Str("action", step).Uint32("uid", uid).Msg("Failed to apply warmup action (IMAP)")
		}
	}
}

func lookupMailboxByUIDValidity(boxes []*models.Mailbox, uidValidity uint32) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && b.UIDValidity == uidValidity {
			return b
		}
	}
	return nil
}

func lookupInbox(boxes []*models.Mailbox) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && imap.IsInboxMailbox(b.Name, b.Attrs) {
			return b
		}
	}
	return nil
}
