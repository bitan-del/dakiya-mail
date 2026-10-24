package tasks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/generation"
	"github.com/bitan-del/dakiya-mail/internal/pkg/warmupwriter"
	"github.com/bitan-del/dakiya-mail/internal/repository"
)

// writerProvider is a scripted LLM provider. Only Complete is used by the writer.
type writerProvider struct {
	generation.Provider
	mu      sync.Mutex
	replies []string
	err     error
	calls   int
	prompt  string
}

func (p *writerProvider) Complete(_ context.Context, req generation.CompletionRequest) (*generation.WritingResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.prompt = req.Prompt
	if p.err != nil {
		return nil, p.err
	}
	i := p.calls - 1
	if i >= len(p.replies) {
		i = len(p.replies) - 1
	}
	return &generation.WritingResult{Text: p.replies[i]}, nil
}

func (p *writerProvider) ModelForTier(bool) string { return "fake-model" }

// threadStore is an in-memory warmup thread repository.
type threadStore struct {
	mu      sync.Mutex
	msgs    []repository.WarmupThreadMessage
	readErr error
}

func (s *threadStore) Save(_ context.Context, m *repository.WarmupThreadMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *m
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	s.msgs = append(s.msgs, cp)
	return nil
}

func (s *threadStore) Thread(_ context.Context, id uuid.UUID, upto, limit int) ([]repository.WarmupThreadMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return nil, s.readErr
	}
	var out []repository.WarmupThreadMessage
	for _, m := range s.msgs {
		if m.ThreadID == id && m.Turn <= upto {
			out = append(out, m)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *threadStore) RecentSentBodies(_ context.Context, id uuid.UUID, _ int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return nil, s.readErr
	}
	var out []string
	for _, m := range s.msgs {
		if m.SenderAccountID == id {
			out = append(out, m.Body)
		}
	}
	return out, nil
}

func (s *threadStore) PurgeBefore(context.Context, time.Time) (int64, error) { return 0, nil }

// settingsRepo serves fixed generation settings and an empty bank, so the
// fallback path lands on the static library.
type settingsRepo struct {
	repository.WarmupContentRepository
	settings models.WarmupGenerationSettings
}

func (r *settingsRepo) GetGenerationSettings(context.Context) (*models.WarmupGenerationSettings, error) {
	s := r.settings
	return &s, nil
}

func (r *settingsRepo) PickConversation(context.Context, string) (*models.WarmupConversation, error) {
	return nil, nil
}

const (
	writerOpeningA = "Subject: Sourdough saga\n\nSo my loaf came out flat as a pancake this weekend, which is the third time in a row. I think the starter is just tired, or maybe my kitchen is too cold. Anyway, I'm trying again on Saturday with the oven light on. Have you ever managed to get one to rise properly?"
	writerOpeningB = "Subject: Saturday hike\n\nWe finally did the ridge walk behind the reservoir and it was windier than the forecast said. Took about three hours with a long stop for sandwiches. My knees are complaining today. Are you around next weekend if we try the longer loop?"
	writerReplyC   = "Honestly the oven light trick worked for me last winter, I just left the dough in there overnight with a bowl of warm water next to it. Mine still came out a bit dense but it did rise. Try a smaller batch first so you're not wasting flour."
)

func writerAccount(name string) *Email {
	org := uuid.New()
	return &Email{ID: uuid.New(), OrganizationID: &org, Name: name, Email: strings.ToLower(name) + "@example.com", Status: "active"}
}

func liveSettings(percent int) models.WarmupGenerationSettings {
	s := models.DefaultWarmupGenerationSettings()
	s.LiveWriterPercent = percent
	return s
}

func TestWriteOpeningFallsBackWhenTheWriterIsNotOnOrNotAvailable(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	killed := liveSettings(100)
	killed.LiveWriterKillSwitch = true

	cases := map[string]struct {
		svc      *tasksService
		settings models.WarmupGenerationSettings
	}{
		"percent zero":    {&tasksService{aiProvider: &writerProvider{replies: []string{writerOpeningA}}, warmupThreads: &threadStore{}}, liveSettings(0)},
		"kill switch":     {&tasksService{aiProvider: &writerProvider{replies: []string{writerOpeningA}}, warmupThreads: &threadStore{}}, killed},
		"no provider":     {&tasksService{warmupThreads: &threadStore{}}, liveSettings(100)},
		"no thread store": {&tasksService{aiProvider: &writerProvider{replies: []string{writerOpeningA}}}, liveSettings(100)},
		"provider error":  {&tasksService{aiProvider: &writerProvider{err: errors.New("down")}, warmupThreads: &threadStore{}}, liveSettings(100)},
		"invalid output":  {&tasksService{aiProvider: &writerProvider{replies: []string{"Subject: Nope\n\nsee https://example.org now please friend"}}, warmupThreads: &threadStore{}}, liveSettings(100)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if d := c.svc.writeOpening(context.Background(), c.settings, sender, partner); d != nil {
				t.Errorf("writer produced %q, want a fall back to the bank", d.body)
			}
		})
	}
}

func TestWriteOpeningProducesAThreadDraft(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	prov := &writerProvider{replies: []string{writerOpeningA}}
	svc := &tasksService{aiProvider: prov, warmupThreads: &threadStore{}}

	d := svc.writeOpening(context.Background(), liveSettings(100), sender, partner)
	if d == nil {
		t.Fatal("no draft")
	}
	if d.turn != 0 || d.threadID == uuid.Nil || d.subject != "Sourdough saga" {
		t.Errorf("draft = %+v", d)
	}
	if !strings.Contains(d.body, "Priya") || !strings.HasSuffix(d.body, "\nJordan") {
		t.Errorf("body not written for the persona and recipient:\n%s", d.body)
	}
}

func TestPickOpeningContentFallsBackToTheBankPathAndNeverStalls(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	repo := &settingsRepo{settings: liveSettings(100)}

	failing := &tasksService{aiProvider: &writerProvider{err: errors.New("timeout")}, warmupThreads: &threadStore{}, warmupContentRepo: repo, warmupSettings: &warmupSettingsCache{}}
	got := failing.pickOpeningContent(context.Background(), sender, partner)
	if got.contentSource != models.WarmupContentSourceStatic || got.body == "" || got.subject == "" {
		t.Errorf("fallback = %+v, want the existing static content", got)
	}

	working := &tasksService{aiProvider: &writerProvider{replies: []string{writerOpeningA}}, warmupThreads: &threadStore{}, warmupContentRepo: repo, warmupSettings: &warmupSettingsCache{}}
	got = working.pickOpeningContent(context.Background(), sender, partner)
	if got.contentSource != models.WarmupContentSourceWriter || got.conversationID == nil || got.theme != writerTheme {
		t.Fatalf("writer content = %+v", got)
	}
	// The stock opener line is the bank renderer's; writer output must not get it.
	for _, opener := range []string{"Hope your week is going well.", "Hope you're doing well.", "Hope things are good on your end."} {
		if strings.Contains(got.body, opener) {
			t.Errorf("writer body carries the canned opener %q", opener)
		}
	}
}

func TestPickOpeningContentStaysOnTheBankWhileTheWriterIsDark(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	prov := &writerProvider{replies: []string{writerOpeningA}}
	svc := &tasksService{aiProvider: prov, warmupThreads: &threadStore{}, warmupContentRepo: &settingsRepo{settings: models.DefaultWarmupGenerationSettings()}, warmupSettings: &warmupSettingsCache{}}

	if got := svc.pickOpeningContent(context.Background(), sender, partner); got.contentSource != models.WarmupContentSourceStatic {
		t.Errorf("source = %q at the default 0%%", got.contentSource)
	}
	if prov.calls != 0 {
		t.Errorf("a dark writer made %d model calls", prov.calls)
	}
}

func storedThread(thread uuid.UUID, from, to *Email, turn int, subject, body string) repository.WarmupThreadMessage {
	return repository.WarmupThreadMessage{
		ThreadID: thread, Turn: turn, Direction: repository.WarmupThreadDirection(turn),
		SenderAccountID: from.ID, RecipientAccountID: to.ID, Subject: subject, Body: body,
	}
}

func TestWriteReplyAnswersTheStoredMessage(t *testing.T) {
	me, partner := writerAccount("Priya"), writerAccount("Jordan")
	thread := uuid.New()
	store := &threadStore{}
	_ = store.Save(context.Background(), writerPtr(storedThread(thread, partner, me, 0, "Sourdough saga", strings.TrimPrefix(writerOpeningA, "Subject: Sourdough saga\n\n"))))
	prov := &writerProvider{replies: []string{writerReplyC}}
	svc := &tasksService{aiProvider: prov, warmupThreads: store}
	cand := &repository.WarmupReplyCandidate{ContentSource: models.WarmupContentSourceWriter, ConversationID: &thread, ConversationTurn: 0}

	d := svc.writeReply(context.Background(), liveSettings(0), me, partner, cand)
	if d == nil {
		t.Fatal("no reply draft")
	}
	if d.turn != 1 || d.threadID != thread || d.subject != "" {
		t.Errorf("draft = %+v, want turn 1 on the same thread with no new subject", d)
	}
	if !strings.Contains(prov.prompt, "oven light on") || !strings.Contains(prov.prompt, "[them]") {
		t.Errorf("the writer was not shown the message it answers:\n%s", prov.prompt)
	}
	if !strings.Contains(d.body, "oven light trick") {
		t.Errorf("body = %s", d.body)
	}
}

func TestWriteReplyReturnsNilWhenItCannotAnswerTheRealMessage(t *testing.T) {
	me, partner, other := writerAccount("Priya"), writerAccount("Jordan"), writerAccount("Sam")
	thread := uuid.New()
	body := strings.TrimPrefix(writerOpeningA, "Subject: Sourdough saga\n\n")
	ok := func() *threadStore {
		s := &threadStore{}
		_ = s.Save(context.Background(), writerPtr(storedThread(thread, partner, me, 0, "Sourdough saga", body)))
		return s
	}
	cand := func(turn int) *repository.WarmupReplyCandidate {
		return &repository.WarmupReplyCandidate{ContentSource: models.WarmupContentSourceWriter, ConversationID: &thread, ConversationTurn: turn}
	}
	killed := liveSettings(100)
	killed.LiveWriterKillSwitch = true
	prov := func() *writerProvider { return &writerProvider{replies: []string{writerReplyC}} }

	cases := map[string]struct {
		svc      *tasksService
		settings models.WarmupGenerationSettings
		cand     *repository.WarmupReplyCandidate
		partner  *Email
	}{
		"message not stored":   {&tasksService{aiProvider: prov(), warmupThreads: &threadStore{}}, liveSettings(100), cand(0), partner},
		"latest turn differs":  {&tasksService{aiProvider: prov(), warmupThreads: ok()}, liveSettings(100), cand(2), partner},
		"sent by someone else": {&tasksService{aiProvider: prov(), warmupThreads: ok()}, liveSettings(100), cand(0), other},
		"thread is finished":   {&tasksService{aiProvider: prov(), warmupThreads: ok()}, liveSettings(100), cand(warmupwriter.MaxTurn), partner},
		"store unreadable":     {&tasksService{aiProvider: prov(), warmupThreads: &threadStore{readErr: errors.New("db")}}, liveSettings(100), cand(0), partner},
		"kill switch":          {&tasksService{aiProvider: prov(), warmupThreads: ok()}, killed, cand(0), partner},
		"no provider":          {&tasksService{warmupThreads: ok()}, liveSettings(100), cand(0), partner},
		"provider error":       {&tasksService{aiProvider: &writerProvider{err: errors.New("down")}, warmupThreads: ok()}, liveSettings(100), cand(0), partner},
		"no thread id":         {&tasksService{aiProvider: prov(), warmupThreads: ok()}, liveSettings(100), &repository.WarmupReplyCandidate{ContentSource: models.WarmupContentSourceWriter}, partner},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if d := c.svc.writeReply(context.Background(), c.settings, me, c.partner, c.cand); d != nil {
				t.Errorf("replied with %q", d.body)
			}
		})
	}
}

func TestWriteReplyFollowsAThreadEvenWhileNewThreadsAreDark(t *testing.T) {
	me, partner := writerAccount("Priya"), writerAccount("Jordan")
	thread := uuid.New()
	store := &threadStore{}
	_ = store.Save(context.Background(), writerPtr(storedThread(thread, partner, me, 0, "Sourdough saga", strings.TrimPrefix(writerOpeningA, "Subject: Sourdough saga\n\n"))))
	svc := &tasksService{aiProvider: &writerProvider{replies: []string{writerReplyC}}, warmupThreads: store}
	cand := &repository.WarmupReplyCandidate{ContentSource: models.WarmupContentSourceWriter, ConversationID: &thread}

	if d := svc.writeReply(context.Background(), liveSettings(0), me, partner, cand); d == nil {
		t.Error("percent only gates new threads; a reply on a writer thread follows the kill switch alone")
	}
}

func TestWriterRegeneratesWhenTheMailboxWouldRepeatItself(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	store := &threadStore{}
	_ = store.Save(context.Background(), writerPtr(storedThread(uuid.New(), sender, partner, 0, "Sourdough saga", strings.TrimPrefix(writerOpeningA, "Subject: Sourdough saga\n\n"))))

	prov := &writerProvider{replies: []string{writerOpeningA, writerOpeningB}}
	d := (&tasksService{aiProvider: prov, warmupThreads: store}).writeOpening(context.Background(), liveSettings(100), sender, partner)
	if d == nil || d.subject != "Saturday hike" || prov.calls != 2 {
		t.Fatalf("draft = %+v after %d calls, want the regenerated second draft", d, prov.calls)
	}

	stuck := &writerProvider{replies: []string{writerOpeningA}}
	if d := (&tasksService{aiProvider: stuck, warmupThreads: store}).writeOpening(context.Background(), liveSettings(100), sender, partner); d != nil {
		t.Error("a second repeat must fall back to the bank, not send a duplicate")
	}
	if stuck.calls != 2 {
		t.Errorf("calls = %d, want one regeneration only", stuck.calls)
	}
}

func TestWriterGuardIsBestEffortWhenHistoryCannotBeRead(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	svc := &tasksService{aiProvider: &writerProvider{replies: []string{writerOpeningA}}, warmupThreads: &threadStore{readErr: errors.New("db")}}
	if d := svc.writeOpening(context.Background(), liveSettings(100), sender, partner); d == nil {
		t.Error("an unreadable history must not stop the writer")
	}
}

func TestRecordWarmupThreadStoresTheSentMessage(t *testing.T) {
	sender, partner := writerAccount("Jordan"), writerAccount("Priya")
	store := &threadStore{}
	svc := &tasksService{warmupThreads: store}
	thread := uuid.New()

	svc.recordWarmupThread(context.Background(), thread, 1, sender, partner, "Re: Sourdough saga", "Try a smaller batch.")

	if len(store.msgs) != 1 {
		t.Fatalf("stored %d messages", len(store.msgs))
	}
	m := store.msgs[0]
	if m.ThreadID != thread || m.Turn != 1 || m.Direction != repository.WarmupThreadResponder || m.SenderAccountID != sender.ID || m.RecipientAccountID != partner.ID || m.Body != "Try a smaller batch." {
		t.Errorf("stored = %+v", m)
	}
	(&tasksService{}).recordWarmupThread(context.Background(), thread, 0, sender, partner, "s", "b") // no store: no panic
}

func writerPtr[T any](v T) *T { return &v }
