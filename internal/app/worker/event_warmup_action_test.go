package worker

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bitan-del/dakiya-mail/internal/client/smtpimap/imap"
)

func TestPlanImapWarmup(t *testing.T) {
	tests := []struct {
		name        string
		actions     []string
		spam        bool
		canResolve  bool
		wantSteps   []string
		wantUnknown []string
	}{
		{"immediate leg from INBOX files only", []string{"move_to_dakiya", "remove_from_spam"}, false, true, []string{"move_to_dakiya"}, nil},
		{"immediate leg from Junk rescues before filing", []string{"move_to_dakiya", "remove_from_spam"}, true, true, []string{"remove_from_spam", "move_to_dakiya"}, nil},
		{"Junk without a Message-ID keeps the rescue and skips filing", []string{"move_to_dakiya", "remove_from_spam"}, true, false, []string{"remove_from_spam"}, nil},
		{"flags run before any move", []string{"move_to_dakiya", "mark_important", "mark_read", "remove_from_spam"}, true, true, []string{"mark_read", "mark_important", "remove_from_spam", "move_to_dakiya"}, nil},
		{"delayed leg is flags only and star is a no-op", []string{"mark_read", "mark_important", "star"}, false, true, []string{"mark_read", "mark_important"}, nil},
		{"star alone plans nothing", []string{"star"}, false, true, nil, nil},
		{"rescue outside a spam folder plans nothing", []string{"remove_from_spam"}, false, true, nil, nil},
		{"filing from INBOX needs no Message-ID", []string{"move_to_dakiya"}, false, false, []string{"move_to_dakiya"}, nil},
		{"unknown actions are reported, not run", []string{"mark_read", "bogus"}, false, true, []string{"mark_read"}, []string{"bogus"}},
		{"empty", nil, true, true, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, unknown := planImapWarmup(tt.actions, tt.spam, tt.canResolve)
			if !slices.Equal(steps, tt.wantSteps) {
				t.Fatalf("steps = %v, want %v", steps, tt.wantSteps)
			}
			if !slices.Equal(unknown, tt.wantUnknown) {
				t.Fatalf("unknown = %v, want %v", unknown, tt.wantUnknown)
			}
		})
	}
}

func TestImapLookupOrder(t *testing.T) {
	tests := []struct {
		source, inbox string
		want          []string
	}{
		{"INBOX", "INBOX", []string{imap.WarmupFolderName, "INBOX"}},
		{"Junk", "INBOX", []string{imap.WarmupFolderName, "Junk", "INBOX"}},
		{imap.WarmupFolderName, "INBOX", []string{imap.WarmupFolderName, "INBOX"}},
	}
	for _, tt := range tests {
		if got := imapLookupOrder(tt.source, tt.inbox); !slices.Equal(got, tt.want) {
			t.Errorf("imapLookupOrder(%q, %q) = %v, want %v", tt.source, tt.inbox, got, tt.want)
		}
	}
}

// fakeIMAP models one message on a server that, like a real one, silently
// accepts a STORE or MOVE against a UID that is no longer in that mailbox.
type fakeIMAP struct {
	mailbox   string
	uid       uint32
	seen      bool
	flagged   bool
	visited   []string // every mailbox the message has been in, in order
	calls     []string
	nextUID   uint32
	rescueErr error
	findErr   error
}

func newFakeIMAP(mailbox string, uid uint32) *fakeIMAP {
	return &fakeIMAP{mailbox: mailbox, uid: uid, visited: []string{mailbox}, nextUID: 1000}
}

func (f *fakeIMAP) at(mailbox string, uid uint32) bool { return f.mailbox == mailbox && f.uid == uid }

func (f *fakeIMAP) move(dst string) {
	f.mailbox, f.uid = dst, f.nextUID
	f.nextUID++
	f.visited = append(f.visited, dst)
}

func (f *fakeIMAP) MarkAsRead(_ context.Context, mailbox string, uid uint32) error {
	f.calls = append(f.calls, "mark_read")
	if f.at(mailbox, uid) {
		f.seen = true
	}
	return nil
}

func (f *fakeIMAP) MarkImportant(_ context.Context, mailbox string, uid uint32) error {
	f.calls = append(f.calls, "mark_important")
	if f.at(mailbox, uid) {
		f.flagged = true
	}
	return nil
}

func (f *fakeIMAP) MoveToFolder(_ context.Context, src, dst string, uid uint32) error {
	f.calls = append(f.calls, "move_to_dakiya")
	if f.at(src, uid) {
		f.move(dst)
	}
	return nil
}

func (f *fakeIMAP) RemoveFromSpam(_ context.Context, src, inbox string, uid uint32) error {
	f.calls = append(f.calls, "remove_from_spam")
	if f.rescueErr != nil {
		return f.rescueErr
	}
	if f.at(src, uid) {
		f.move(inbox)
	}
	return nil
}

func (f *fakeIMAP) FindByMessageID(_ context.Context, _ string, mailboxes []string) (string, uint32, error) {
	f.calls = append(f.calls, "find")
	if f.findErr != nil {
		return "", 0, f.findErr
	}
	if slices.Contains(mailboxes, f.mailbox) {
		return f.mailbox, f.uid, nil
	}
	return "", 0, nil
}

// runLeg plans and applies one leg the way runImapWarmupActions does.
func runLeg(f *fakeIMAP, actions []string, sourceName string, sourceIsSpam bool, uid uint32, rfcID string) {
	steps, _ := planImapWarmup(actions, sourceIsSpam, rfcID != "")
	applyImapWarmup(context.Background(), f, steps, sourceName, "INBOX", uid, rfcID)
}

func TestApplyImapWarmup(t *testing.T) {
	const rfcID = "<abc@sender.example>"
	immediate := []string{"move_to_dakiya", "remove_from_spam"}
	delayed := []string{"mark_read", "mark_important", "star"}

	t.Run("filing from INBOX lands in the Dakiya Mail folder", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		runLeg(f, immediate, "INBOX", false, 7, rfcID)
		if f.mailbox != imap.WarmupFolderName {
			t.Fatalf("message in %q, want %q", f.mailbox, imap.WarmupFolderName)
		}
		if slices.Contains(f.calls, "find") {
			t.Fatalf("a leg that starts at the source must not search: %v", f.calls)
		}
	})

	t.Run("Junk is rescued through INBOX before filing", func(t *testing.T) {
		f := newFakeIMAP("Junk", 7)
		runLeg(f, immediate, "Junk", true, 7, rfcID)
		if want := []string{"Junk", "INBOX", imap.WarmupFolderName}; !slices.Equal(f.visited, want) {
			t.Fatalf("path = %v, want %v", f.visited, want)
		}
	})

	t.Run("Junk rescue without a Message-ID still reaches INBOX", func(t *testing.T) {
		f := newFakeIMAP("Junk", 7)
		runLeg(f, immediate, "Junk", true, 7, "")
		if want := []string{"Junk", "INBOX"}; !slices.Equal(f.visited, want) {
			t.Fatalf("path = %v, want %v", f.visited, want)
		}
	})

	t.Run("failed rescue still files from Junk", func(t *testing.T) {
		f := newFakeIMAP("Junk", 7)
		f.rescueErr = errors.New("boom")
		runLeg(f, immediate, "Junk", true, 7, rfcID)
		if f.mailbox != imap.WarmupFolderName {
			t.Fatalf("message in %q, want %q", f.mailbox, imap.WarmupFolderName)
		}
	})

	t.Run("flags in the same leg land before the move", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		runLeg(f, append(slices.Clone(immediate), "mark_read", "mark_important"), "INBOX", false, 7, rfcID)
		if !f.seen || !f.flagged || f.mailbox != imap.WarmupFolderName {
			t.Fatalf("seen=%v flagged=%v mailbox=%q", f.seen, f.flagged, f.mailbox)
		}
		if f.calls[0] != "mark_read" || f.calls[1] != "mark_important" {
			t.Fatalf("flags must precede the move: %v", f.calls)
		}
	})

	t.Run("delayed leg finds the filed message under its new UID", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		runLeg(f, immediate, "INBOX", false, 7, rfcID)
		runLeg(f, delayed, "INBOX", false, 7, rfcID) // payload still carries the old UID 7
		if !f.seen || !f.flagged {
			t.Fatalf("seen=%v flagged=%v: flags never reached the moved message", f.seen, f.flagged)
		}
	})

	t.Run("delayed leg finds a message that was rescued but never filed", func(t *testing.T) {
		f := newFakeIMAP("Junk", 7)
		runLeg(f, []string{"remove_from_spam"}, "Junk", true, 7, rfcID)
		runLeg(f, delayed, "Junk", true, 7, rfcID)
		if !f.seen || !f.flagged || f.mailbox != "INBOX" {
			t.Fatalf("seen=%v flagged=%v mailbox=%q", f.seen, f.flagged, f.mailbox)
		}
	})

	t.Run("delayed leg flags a message that never moved", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		runLeg(f, delayed, "INBOX", false, 7, rfcID)
		if !f.seen || !f.flagged {
			t.Fatalf("seen=%v flagged=%v", f.seen, f.flagged)
		}
	})

	t.Run("delayed leg does nothing when the message is gone", func(t *testing.T) {
		f := newFakeIMAP("Trash", 7)
		runLeg(f, delayed, "INBOX", false, 7, rfcID)
		if want := []string{"find"}; !slices.Equal(f.calls, want) {
			t.Fatalf("calls = %v, want %v", f.calls, want)
		}
	})

	t.Run("delayed leg does nothing when the search fails", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		f.findErr = errors.New("boom")
		runLeg(f, delayed, "INBOX", false, 7, rfcID)
		if f.seen || f.flagged {
			t.Fatal("flags applied without locating the message")
		}
	})

	t.Run("delayed leg without a Message-ID falls back to the source UID", func(t *testing.T) {
		f := newFakeIMAP("INBOX", 7)
		runLeg(f, delayed, "INBOX", false, 7, "")
		if !f.seen || !f.flagged || slices.Contains(f.calls, "find") {
			t.Fatalf("seen=%v flagged=%v calls=%v", f.seen, f.flagged, f.calls)
		}
	})
}
