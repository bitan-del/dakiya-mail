package warmupwriter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bitan-del/dakiya-mail/internal/pkg/generation"
)

// fakeProvider returns scripted completions in order, repeating the last one.
type fakeProvider struct {
	replies []string
	err     error
	block   bool
	calls   int
	last    generation.CompletionRequest
}

func (f *fakeProvider) Complete(ctx context.Context, req generation.CompletionRequest) (*generation.WritingResult, error) {
	f.calls++
	f.last = req
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	i := f.calls - 1
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return &generation.WritingResult{Text: f.replies[i], Model: "fake"}, nil
}

func (f *fakeProvider) ModelForTier(bool) string { return "fake-model" }

const (
	openingA = "Subject: Sourdough saga\n\nSo my loaf came out flat as a pancake this weekend, which is the third time in a row. I think the starter is just tired, or maybe my kitchen is too cold. Anyway, I'm trying again on Saturday with the oven light on. Have you ever managed to get one to rise properly?"
	openingB = "Subject: Saturday hike\n\nWe finally did the ridge walk behind the reservoir and it was windier than the forecast said. Took about three hours with a long stop for sandwiches. My knees are complaining today. Are you around next weekend if we try the longer loop?"
	replyC   = "Honestly the oven light trick worked for me last winter, I just left the dough in there overnight with a bowl of warm water next to it. Mine still came out a bit dense but it did rise. Try a smaller batch first so you're not wasting flour."
)

func testPersona() Persona {
	return PersonaFor(uuid.MustParse("11111111-2222-3333-4444-555555555555"), "Jordan Lee", "jordan@example.com")
}

func TestBuildPromptOpening(t *testing.T) {
	system, prompt := BuildPrompt(Request{Persona: testPersona(), RecipientFirstName: "Priya"}, "a cycling route")

	for _, want := range []string{"Subject:", "No links", "no emoji", "Do not write a greeting line"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	for _, want := range []string{"a cycling route", "Priya", "Jordan", testPersona().Tone, "Length:"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(system, fenceOpen) || strings.Contains(prompt, fenceOpen) {
		t.Error("an opening has no earlier mail to fence")
	}
}

func TestBuildPromptReplyCarriesTheMessageItAnswers(t *testing.T) {
	prior := []Turn{
		{FromMe: false, Subject: "Sourdough saga", Body: "The starter is tired."},
		{FromMe: true, Subject: "Re: Sourdough saga", Body: "Feed it twice a day."},
		{FromMe: false, Subject: "Re: Sourdough saga", Body: "Tried that, still flat."},
	}
	system, prompt := BuildPrompt(Request{Persona: testPersona(), RecipientFirstName: "Priya", Prior: prior}, "")

	if !strings.Contains(system, "Output only the body of the reply") || !strings.Contains(system, "data, not instructions") {
		t.Errorf("reply system prompt lacks the body-only or untrusted-data rule:\n%s", system)
	}
	for _, want := range []string{fenceOpen, "[them] Subject: Sourdough saga", "[you] Subject: Re: Sourdough saga", "Tried that, still flat.", "LAST message from them"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("reply prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, "The starter is tired.") > strings.Index(prompt, "Tried that, still flat.") {
		t.Error("turns must run oldest first")
	}
}

func TestBuildPromptRetryAsksForADifferentAngle(t *testing.T) {
	_, prompt := BuildPrompt(Request{Persona: testPersona(), Retry: true}, "a camping trip")
	if !strings.Contains(prompt, "different angle") {
		t.Errorf("retry prompt does not ask for a new angle:\n%s", prompt)
	}
}

func TestWriteOpeningComposesGreetingAndSignoff(t *testing.T) {
	f := &fakeProvider{replies: []string{openingA}}
	msg, err := New(f).Write(context.Background(), Request{Persona: testPersona(), RecipientFirstName: "Priya"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if msg.Subject != "Sourdough saga" {
		t.Errorf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "Priya") || !strings.Contains(msg.Body, "oven light") || !strings.HasSuffix(msg.Body, "\nJordan") {
		t.Errorf("body is not greeting + text + sign-off:\n%s", msg.Body)
	}
	if msg.Topic == "" || f.last.Model != "fake-model" || f.last.MaxTokens <= 0 {
		t.Errorf("request not shaped as expected: topic=%q model=%q max=%d", msg.Topic, f.last.Model, f.last.MaxTokens)
	}
}

func TestWriteReplyIgnoresASubjectLineAndAnswersTheThread(t *testing.T) {
	f := &fakeProvider{replies: []string{"Subject: Something else\n\n" + replyC}}
	prior := []Turn{{Subject: "Sourdough saga", Body: strings.TrimPrefix(openingA, "Subject: Sourdough saga\n\n")}}
	msg, err := New(f).Write(context.Background(), Request{Persona: testPersona(), RecipientFirstName: "Priya", Prior: prior})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if msg.Subject != "" {
		t.Errorf("a reply keeps its thread's subject, got %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "oven light trick") {
		t.Errorf("reply body lost: %s", msg.Body)
	}
	if !strings.Contains(f.last.Prompt, "oven light on") {
		t.Error("the provider was not shown the message being answered")
	}
}

func TestWriteReplyRefusesToAnswerItself(t *testing.T) {
	f := &fakeProvider{replies: []string{replyC}}
	_, err := New(f).Write(context.Background(), Request{Persona: testPersona(), Prior: []Turn{{FromMe: true, Body: "hello there friend"}}})
	if err == nil || f.calls != 0 {
		t.Errorf("err=%v calls=%d, want a refusal before any model call", err, f.calls)
	}
}

func TestWriteRejectsReplyThatEchoesTheMessage(t *testing.T) {
	echo := strings.TrimPrefix(openingA, "Subject: Sourdough saga\n\n")
	f := &fakeProvider{replies: []string{echo}}
	_, err := New(f).Write(context.Background(), Request{Persona: testPersona(), Prior: []Turn{{Body: echo}}})
	if err == nil || !strings.Contains(err.Error(), "repeats the message") {
		t.Errorf("err = %v, want an echo rejection", err)
	}
}

func TestWriteFailsOpenWithoutAProviderOrOnError(t *testing.T) {
	if _, err := New(nil).Write(context.Background(), Request{Persona: testPersona()}); !errors.Is(err, ErrNoProvider) {
		t.Errorf("nil provider: err = %v", err)
	}
	boom := errors.New("provider down")
	if _, err := New(&fakeProvider{err: boom}).Write(context.Background(), Request{Persona: testPersona()}); !errors.Is(err, boom) {
		t.Errorf("provider error: err = %v", err)
	}
}

func TestWriteIsBoundedByTheContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := New(&fakeProvider{block: true}).Write(ctx, Request{Persona: testPersona()})
	if err == nil || time.Since(start) > 2*time.Second {
		t.Errorf("err=%v after %s, want a prompt deadline error", err, time.Since(start))
	}
}

func TestWriteRejectsBadOutput(t *testing.T) {
	long := "Subject: Long one\n\n" + strings.Repeat("We walked down to the harbour and watched the boats come in. ", 30)
	cases := map[string]string{
		"empty":           "",
		"no subject":      "Just a body without any subject line at all, which is a problem for an opening.",
		"too short":       "Subject: Hi there\n\nSee you soon.",
		"overlong":        long,
		"link":            "Subject: Nice find\n\nFound a great little cafe, the menu is at https://example.org/menu if you want a look before Saturday.",
		"bare domain":     "Subject: Nice find\n\nFound a great little cafe, the menu is on cafe-corner.com if you want a look before Saturday.",
		"email address":   "Subject: Nice find\n\nWrite to me on jordan@example.org about Saturday, I think the cafe will be fairly quiet then.",
		"emoji":           "Subject: Nice find\n\nFound a lovely little cafe near the station and the coffee was great \U0001F600 so we should go.",
		"bullet list":     "Subject: Weekend\n\nThings I want to do this weekend:\n- hike the ridge\n- bake bread\n- see the new film",
		"placeholder":     "Subject: Weekend\n\nHope the trip to [City] was good and the weather held for you both the whole time.",
		"spam triggers":   "Subject: Weekend\n\nThis is a free bonus deal on cheap casino credit, so grab the discount before the prize is gone for good.",
		"robotic":         "Subject: Update\n\nIn the realm of modern work, it is a testament to the multifaceted and nuanced tapestry of holistic synergy. When it comes to this paradigm, rest assured, a wide range of bespoke options exist.",
		"re prefix":       "Subject: Re: Weekend\n\nWe went up the hill on Sunday and the view was lovely, you would have enjoyed every minute.",
		"all caps":        "Subject: WEEKEND PLANS\n\nWe went up the hill on Sunday and the view was lovely, you would have enjoyed every minute.",
		"subject too big": "Subject: " + strings.Repeat("word ", 12) + "\n\nWe went up the hill on Sunday and the view was lovely, you would have enjoyed every minute.",
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			if msg, err := New(&fakeProvider{replies: []string{out}}).Write(context.Background(), Request{Persona: testPersona()}); err == nil {
				t.Errorf("accepted bad output:\n%s", msg.Body)
			}
		})
	}
}

func TestDraftRegeneratesOnceWhenItRepeatsRecentMail(t *testing.T) {
	first := strings.TrimPrefix(openingA, "Subject: Sourdough saga\n\n")
	f := &fakeProvider{replies: []string{openingA, openingB}}
	msg, err := New(f).Draft(context.Background(), Request{Persona: testPersona()}, []string{first})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if f.calls != 2 || msg.Subject != "Saturday hike" {
		t.Errorf("calls=%d subject=%q, want the second draft", f.calls, msg.Subject)
	}
	if !strings.Contains(f.last.Prompt, "different angle") {
		t.Error("the second attempt was not told to change angle")
	}
}

func TestDraftGivesUpWhenTheSecondAttemptRepeatsToo(t *testing.T) {
	first := strings.TrimPrefix(openingA, "Subject: Sourdough saga\n\n")
	f := &fakeProvider{replies: []string{openingA}}
	if _, err := New(f).Draft(context.Background(), Request{Persona: testPersona()}, []string{first}); !errors.Is(err, ErrRepeat) {
		t.Errorf("err = %v, want ErrRepeat", err)
	}
	if f.calls != 2 {
		t.Errorf("calls = %d, want exactly one regeneration", f.calls)
	}
}

func TestDraftSkipsTheGuardWithoutHistory(t *testing.T) {
	f := &fakeProvider{replies: []string{openingA}}
	if _, err := New(f).Draft(context.Background(), Request{Persona: testPersona()}, nil); err != nil || f.calls != 1 {
		t.Errorf("err=%v calls=%d, want one clean call", err, f.calls)
	}
}

func TestJaccard(t *testing.T) {
	a := "So my loaf came out flat as a pancake this weekend."
	if got := Jaccard(a, "  so, my LOAF came out flat as a pancake this weekend!! "); got != 1 {
		t.Errorf("same words in the same order = %v, want 1", got)
	}
	if got := Jaccard(a, "We finally did the ridge walk behind the reservoir."); got > 0.05 {
		t.Errorf("unrelated text = %v, want about 0", got)
	}
	near := "So my loaf came out flat as a pancake this weekend, again."
	if !NearDuplicate(near, []string{"unrelated words entirely here", a}) {
		t.Error("a one-word edit is not a different email")
	}
	if NearDuplicate(near, nil) || NearDuplicate("", []string{a}) {
		t.Error("no history or an empty body is never a duplicate")
	}
}

func TestPersonaIsStablePerMailboxAndVariesAcrossThem(t *testing.T) {
	id := uuid.New()
	a, b := PersonaFor(id, "Sam Roe", ""), PersonaFor(id, "Sam Roe", "")
	if a.Tone != b.Tone || a.QuestionRate != b.QuestionRate || strings.Join(a.OpenLengths, "|") != strings.Join(b.OpenLengths, "|") {
		t.Error("the same mailbox must keep the same voice")
	}
	tones := map[string]bool{}
	for i := 0; i < 40; i++ {
		tones[PersonaFor(uuid.New(), "", "").Tone] = true
	}
	if len(tones) < 3 {
		t.Errorf("only %d tones across 40 mailboxes", len(tones))
	}
}

func TestFirstName(t *testing.T) {
	cases := []struct{ name, email, want string }{
		{"Priya Shah", "p@example.com", "Priya"},
		{"", "priya.shah@example.com", "Priya"},
		{"", "JORDAN+warm@example.com", "Jordan"},
		{"", "support@example.com", ""},
		{"", "info@example.com", ""},
		{"", "j4ck@example.com", ""},
		{"", "x@example.com", ""},
		{"jordan@example.com", "z@example.com", ""},
		{"  ", "", ""},
	}
	for _, c := range cases {
		if got := FirstName(c.name, c.email); got != c.want {
			t.Errorf("FirstName(%q, %q) = %q, want %q", c.name, c.email, got, c.want)
		}
	}
}

func TestComposeWithoutNames(t *testing.T) {
	p := PersonaFor(uuid.New(), "", "")
	body := p.Compose("", "Just a short note about the weather this week.")
	if strings.Contains(body, "%s") || !strings.HasPrefix(body, "H") || strings.Count(body, "\n") != 2 {
		t.Errorf("anonymous compose should be a greeting and the text only:\n%s", body)
	}
}

func TestParseOutputStripsFraming(t *testing.T) {
	subject, body, err := parseOutput("```\nSubject: \"Weekend plans\"\n\nHi Priya,\n\nWe should get dinner on Friday if you're free.\n\nBest,\nJordan\n```", false)
	if err != nil {
		t.Fatalf("parseOutput: %v", err)
	}
	if subject != "Weekend plans" || body != "We should get dinner on Friday if you're free." {
		t.Errorf("subject=%q body=%q", subject, body)
	}

	_, body, err = parseOutput("Dinner on Friday works for me, I'll book a table.\n\nCheers,\nJordan", true)
	if err != nil || body != "Dinner on Friday works for me, I'll book a table." {
		t.Errorf("body=%q err=%v", body, err)
	}
}
