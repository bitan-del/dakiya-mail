package warmupwriter

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode"

	"github.com/bitan-del/dakiya-mail/internal/pkg/warmpersona"
	"github.com/google/uuid"
)

var (
	tones = []string{
		"warm and chatty",
		"dry and understated",
		"upbeat and quick",
		"thoughtful and a little formal",
		"laid-back with the odd joke",
		"plain and practical",
	}
	openLengths = []string{
		"two short sentences",
		"three or four sentences",
		"one short paragraph of about five sentences",
		"two short paragraphs",
		"three sentences and nothing more",
	}
	replyLengths = []string{
		"one or two sentences",
		"two or three sentences",
		"a short paragraph",
		"three or four sentences",
	}
	// greetingForms and greetingFormsAnon line up by index, so a persona's
	// preferred forms work with or without a first name.
	greetingForms     = []string{"Hi %s,", "Hey %s,", "Hello %s,", "%s,"}
	greetingFormsAnon = []string{"Hi,", "Hey,", "Hello,", "Hi there,"}
	signoffs          = []string{"Best,", "Cheers,", "Talk soon,", "Take care,", "All the best,", "Speak soon,", "Thanks,", "Warmly,"}

	// genericLocalParts are mailbox names that are not a person's first name.
	genericLocalParts = map[string]bool{
		"info": true, "hello": true, "hi": true, "contact": true, "support": true, "admin": true,
		"team": true, "sales": true, "mail": true, "office": true, "help": true, "service": true,
		"billing": true, "accounts": true, "noreply": true, "hey": true, "enquiries": true, "marketing": true,
	}
)

// Persona is one mailbox's stable writing voice, derived from its account id so
// it stays the same message after message while the topic changes.
type Persona struct {
	// FirstName is how the sender signs off; empty when none can be found.
	FirstName    string
	Tone         string
	OpenLengths  []string
	ReplyLengths []string
	// QuestionRate is the chance a message ends by asking something.
	QuestionRate float64
	greetings    []int
	signoffs     []string
}

// PersonaFor derives a mailbox's voice. name and email only supply the sign-off name.
func PersonaFor(accountID uuid.UUID, name, email string) Persona {
	wp := warmpersona.For(accountID)
	p := Persona{
		FirstName:    FirstName(name, email),
		Tone:         tones[wp.Index("tone", len(tones))],
		QuestionRate: wp.Bias("question", 0.25, 0.75),
		greetings:    wp.Subset("greeting", len(greetingForms), 2),
	}
	for _, i := range wp.Subset("open_length", len(openLengths), 2) {
		p.OpenLengths = append(p.OpenLengths, openLengths[i])
	}
	for _, i := range wp.Subset("reply_length", len(replyLengths), 2) {
		p.ReplyLengths = append(p.ReplyLengths, replyLengths[i])
	}
	for _, i := range wp.Subset("signoff", len(signoffs), 3) {
		p.signoffs = append(p.signoffs, signoffs[i])
	}
	return p
}

// Compose wraps the model's text in this persona's greeting and sign-off.
func (p Persona) Compose(recipientFirst, core string) string {
	var greeting string
	switch {
	case len(p.greetings) == 0:
		greeting = "Hi,"
	default:
		i := p.greetings[rand.Intn(len(p.greetings))]
		if recipientFirst != "" {
			greeting = fmt.Sprintf(greetingForms[i], recipientFirst)
		} else {
			greeting = greetingFormsAnon[i]
		}
	}

	var b strings.Builder
	b.WriteString(greeting)
	b.WriteString("\n\n")
	b.WriteString(strings.TrimSpace(core))
	if p.FirstName != "" && len(p.signoffs) > 0 {
		b.WriteString("\n\n")
		b.WriteString(p.signoffs[rand.Intn(len(p.signoffs))])
		b.WriteString("\n")
		b.WriteString(p.FirstName)
	}
	return b.String()
}

// FirstName returns a person's first name from a display name, falling back to
// the start of the mailbox name when it plausibly is one. Empty when unsure,
// so a greeting never addresses "Support" or "Info".
func FirstName(name, email string) string {
	if fields := strings.Fields(name); len(fields) > 0 {
		if f := cleanName(fields[0]); f != "" {
			return f
		}
		return ""
	}
	local, _, _ := strings.Cut(email, "@")
	local, _, _ = strings.Cut(local, "+")
	first := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	if len(first) == 0 {
		return ""
	}
	f := strings.ToLower(first[0])
	if genericLocalParts[f] || len(f) < 2 || len(f) > 14 || cleanName(f) != f {
		return ""
	}
	return strings.ToUpper(f[:1]) + f[1:]
}

// cleanName keeps a token only when it is purely letters, so addresses, ids and
// punctuation never end up in a greeting.
func cleanName(tok string) string {
	tok = strings.Trim(tok, ",.;:()\"'")
	if tok == "" {
		return ""
	}
	for _, r := range tok {
		if !unicode.IsLetter(r) && r != '-' && r != '\'' {
			return ""
		}
	}
	return tok
}
