package warmupwriter

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/bitan-del/dakiya-mail/internal/pkg/humanlint"
	"github.com/bitan-del/dakiya-mail/internal/pkg/warmlint"
)

// Bounds on what the writer may produce. Warmup mail is short and personal.
const (
	minBodyWords    = 6
	maxBodyWords    = 170
	maxBodyChars    = 1200
	maxSubjectWords = 8
	maxSubjectChars = 70

	// HistorySize is how many of a mailbox's recent sent bodies a new one is compared with.
	HistorySize = 50
	// NearDuplicateThreshold is the word-shingle Jaccard at which two bodies count as the same email.
	NearDuplicateThreshold = 0.6
	shingleWords           = 3
)

var (
	linkRe        = regexp.MustCompile(`(?i)(https?://|www\.|\b[a-z0-9-]+\.(com|net|org|io|co|app|dev|ai|me|info)\b)`)
	emailAddrRe   = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+`)
	markdownRe    = regexp.MustCompile("(?m)(\\*\\*|__|^#{1,6}\\s|^\\s*[-*•]\\s|^\\s*\\d+[.)]\\s|```)")
	placeholderRe = regexp.MustCompile(`\[[^\]]{1,30}\]|\{\{|\}\}|<[^>\n]{1,30}>`)
	spaceRe       = regexp.MustCompile(`\s+`)

	// ErrRepeat means the writer twice produced something its mailbox already sent.
	ErrRepeat = errors.New("warmupwriter: draft repeats a recent message")
)

// Validate rejects output that is empty, overlong, or does not read like a short
// personal email. subject may be empty for a reply, which keeps its thread's subject.
func Validate(subject, body string, isReply bool) error {
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)

	if body == "" {
		return errors.New("empty body")
	}
	words := len(strings.Fields(body))
	switch {
	case words < minBodyWords:
		return fmt.Errorf("body too short (%d words)", words)
	case words > maxBodyWords:
		return fmt.Errorf("body too long (%d words)", words)
	case len(body) > maxBodyChars:
		return fmt.Errorf("body too long (%d chars)", len(body))
	}
	if !isReply {
		switch {
		case subject == "":
			return errors.New("empty subject")
		case len(subject) > maxSubjectChars || len(strings.Fields(subject)) > maxSubjectWords:
			return errors.New("subject too long")
		case strings.ContainsAny(subject, "\r\n"):
			return errors.New("subject spans lines")
		}
	}

	both := subject + "\n" + body
	switch {
	case linkRe.MatchString(both):
		return errors.New("contains a link")
	case emailAddrRe.MatchString(both):
		return errors.New("contains an email address")
	case markdownRe.MatchString(body):
		return errors.New("contains markdown or a list")
	case placeholderRe.MatchString(both):
		return errors.New("contains a placeholder")
	case hasEmoji(both):
		return errors.New("contains an emoji")
	}
	return nil
}

// Lint runs the shared spam and robotic-text gates over a finished message.
func Lint(subject, body string, isReply bool) error {
	if err := warmlint.Check(subject, body, isReply); err != nil {
		return err
	}
	if humanlint.LooksRobotic(body) {
		return errors.New("reads machine-written")
	}
	return nil
}

func hasEmoji(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x1F000 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF, r >= 0x2B00 && r <= 0x2BFF,
			r == 0xFE0F, r == 0x200D:
			return true
		}
	}
	return false
}

// Normalize lowercases and strips punctuation so two bodies compare on their words.
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

func shingles(norm string) map[string]struct{} {
	words := strings.Fields(norm)
	out := map[string]struct{}{}
	if len(words) <= shingleWords {
		if len(words) > 0 {
			out[strings.Join(words, " ")] = struct{}{}
		}
		return out
	}
	for i := 0; i+shingleWords <= len(words); i++ {
		out[strings.Join(words[i:i+shingleWords], " ")] = struct{}{}
	}
	return out
}

// Jaccard is the overlap of the two bodies' three-word shingles, 0 (nothing
// shared) to 1 (the same words in the same order).
func Jaccard(a, b string) float64 {
	return jaccard(shingles(Normalize(a)), shingles(Normalize(b)))
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	small, large := a, b
	if len(small) > len(large) {
		small, large = large, small
	}
	shared := 0
	for k := range small {
		if _, ok := large[k]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

// NearDuplicate reports whether body is nearly identical to any of history.
func NearDuplicate(body string, history []string) bool {
	mine := shingles(Normalize(body))
	for _, h := range history {
		if jaccard(mine, shingles(Normalize(h))) >= NearDuplicateThreshold {
			return true
		}
	}
	return false
}
