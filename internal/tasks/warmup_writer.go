package tasks

import (
	"context"
	"math/rand"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/warmupwriter"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// WarmupThreadsAware is the optional capability the caller uses to give the
// live warmup writer its thread store.
type WarmupThreadsAware interface {
	WireWarmupThreads(r repository.WarmupThreadRepository)
}

// WireWarmupThreads attaches the store the live warmup writer needs. Without it,
// or without an AI provider, warmup content comes from the bank as before.
func (s *tasksService) WireWarmupThreads(r repository.WarmupThreadRepository) {
	s.warmupThreads = r
}

// writerTheme is the conversation theme recorded for a thread the writer opens.
const writerTheme = "writer"

// writerDraft is a message the live writer produced, ready for the send path.
type writerDraft struct {
	subject  string
	body     string
	threadID uuid.UUID
	turn     int
}

func (s *tasksService) liveWriterReady(settings models.WarmupGenerationSettings) bool {
	return s.aiProvider != nil && s.warmupThreads != nil && !settings.LiveWriterKillSwitch
}

// pickOpeningContent writes a new thread live when the writer is on and this
// send is drawn for it, and otherwise (or on any writer failure) takes the
// bank/static content untouched.
func (s *tasksService) pickOpeningContent(ctx context.Context, account, partner *Email) warmupContent {
	if d := s.writeOpening(ctx, s.getWarmupSettings(ctx), account, partner); d != nil {
		id := d.threadID
		return warmupContent{
			subject:        d.subject,
			body:           d.body,
			theme:          writerTheme,
			contentSource:  models.WarmupContentSourceWriter,
			conversationID: &id,
		}
	}
	return s.pickNewWarmupContent(ctx, *account)
}

func (s *tasksService) writeOpening(ctx context.Context, settings models.WarmupGenerationSettings, account, partner *Email) *writerDraft {
	if !s.liveWriterReady(settings) || !settings.LiveWriterOpens(rand.Intn(100)) {
		return nil
	}
	return s.runWriter(ctx, account, partner, nil, uuid.New(), 0)
}

// writeReply answers the real message a writer thread last received. Nil when
// the thread is not one the writer opened, the message is no longer stored, or
// the writer fails; the caller then does not reply on that thread, because a
// pre-written turn would not answer what was said.
func (s *tasksService) writeReply(ctx context.Context, settings models.WarmupGenerationSettings, account, partner *Email, cand *repository.WarmupReplyCandidate) *writerDraft {
	if !s.liveWriterReady(settings) || cand == nil || cand.ConversationID == nil {
		return nil
	}
	turn := cand.ConversationTurn + 1
	if turn > warmupwriter.MaxTurn {
		return nil
	}
	msgs, err := s.warmupThreads.Thread(ctx, *cand.ConversationID, cand.ConversationTurn, 3)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	if last := msgs[len(msgs)-1]; last.Turn != cand.ConversationTurn || last.SenderAccountID != partner.ID {
		return nil
	}
	prior := make([]warmupwriter.Turn, 0, len(msgs))
	for _, m := range msgs {
		prior = append(prior, warmupwriter.Turn{FromMe: m.SenderAccountID == account.ID, Subject: m.Subject, Body: m.Body})
	}
	return s.runWriter(ctx, account, partner, prior, *cand.ConversationID, turn)
}

// runWriter writes one message inside the writer's time budget. A mailbox's own
// recent mail is only a repeat guard, so failing to read it skips the guard.
func (s *tasksService) runWriter(ctx context.Context, account, partner *Email, prior []warmupwriter.Turn, threadID uuid.UUID, turn int) *writerDraft {
	ctx, cancel := context.WithTimeout(ctx, warmupwriter.Timeout)
	defer cancel()

	history, err := s.warmupThreads.RecentSentBodies(ctx, account.ID, warmupwriter.HistorySize)
	if err != nil {
		history = nil
	}
	msg, err := warmupwriter.New(s.aiProvider).Draft(ctx, warmupwriter.Request{
		Persona:            warmupwriter.PersonaFor(account.ID, account.Name, account.Email),
		RecipientFirstName: warmupwriter.FirstName(partner.Name, partner.Email),
		Prior:              prior,
	}, history)
	if err != nil {
		log.Warn().Err(err).
			Str("email_account_id", account.ID.String()).
			Bool("reply", len(prior) > 0).
			Msg("live warmup writer unavailable; using bank content")
		return nil
	}
	return &writerDraft{subject: msg.Subject, body: msg.Body, threadID: threadID, turn: turn}
}

// recordWarmupThread keeps what was sent so the partner's reply can answer it
// and this mailbox can be compared against its own recent mail.
func (s *tasksService) recordWarmupThread(ctx context.Context, threadID uuid.UUID, turn int, account, partner *Email, subject, body string) {
	if s.warmupThreads == nil {
		return
	}
	err := s.warmupThreads.Save(ctx, &repository.WarmupThreadMessage{
		ThreadID:           threadID,
		Turn:               turn,
		Direction:          repository.WarmupThreadDirection(turn),
		SenderAccountID:    account.ID,
		RecipientAccountID: partner.ID,
		Subject:            subject,
		Body:               body,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("email_account_id", account.ID.String()).
			Str("thread_id", threadID.String()).
			Msg("failed to store warmup thread message; the reply will start a new thread")
	}
}
