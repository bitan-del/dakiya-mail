package repository

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

var engNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time {
	t := engNow.Add(-d)
	return &t
}

const day = 24 * time.Hour

func TestEngagementScoreWeighsRepliesOverClicksOverOpens(t *testing.T) {
	reply := engagementSignals{LastReplyAt: ago(day)}.score(engNow)
	click := engagementSignals{LastClickAt: ago(day)}.score(engNow)
	open := engagementSignals{LastOpenAt: ago(day)}.score(engNow)
	if !(reply > click && click > open && open > 0) {
		t.Fatalf("want reply > click > open > 0, got %v %v %v", reply, click, open)
	}
}

func TestEngagementScoreHalvesEveryHalfLife(t *testing.T) {
	fresh := engagementSignals{LastClickAt: ago(0)}.score(engNow)
	aged := engagementSignals{LastClickAt: ago(engagementHalfLife)}.score(engNow)
	if math.Abs(aged-fresh/2) > 1e-9 {
		t.Fatalf("one half-life should halve the score: fresh %v aged %v", fresh, aged)
	}
	// A recent open outranks a reply from many half-lives ago.
	recentOpen := engagementSignals{LastOpenAt: ago(day)}.score(engNow)
	oldReply := engagementSignals{LastReplyAt: ago(400 * day)}.score(engNow)
	if recentOpen <= oldReply {
		t.Fatal("recency should be able to beat weight")
	}
}

func TestEngagementScoreZeroWithoutEngagement(t *testing.T) {
	e := engagementSignals{Sends: 5}
	if e.score(engNow) != 0 || e.engaged() || e.lastEngagedAt() != nil {
		t.Fatalf("a contact who never engaged has no score or last engagement: %+v", e)
	}
}

func TestLastEngagedAtIsTheLatestSignal(t *testing.T) {
	e := engagementSignals{LastReplyAt: ago(10 * day), LastClickAt: ago(2 * day), LastOpenAt: ago(5 * day)}
	got := e.lastEngagedAt()
	if got == nil || !got.Equal(*ago(2 * day)) {
		t.Fatalf("want the click from 2 days ago, got %v", got)
	}
}

func TestSunset(t *testing.T) {
	for _, tc := range []struct {
		name  string
		e     engagementSignals
		after int
		want  bool
	}{
		{"off", engagementSignals{Sends: 99}, 0, false},
		{"below the limit", engagementSignals{Sends: 2}, 3, false},
		{"at the limit with no engagement", engagementSignals{Sends: 3}, 3, true},
		{"past the limit with no engagement", engagementSignals{Sends: 8}, 3, true},
		{"replied", engagementSignals{Sends: 8, LastReplyAt: ago(day)}, 3, false},
		{"clicked", engagementSignals{Sends: 8, LastClickAt: ago(day)}, 3, false},
		{"human open", engagementSignals{Sends: 8, LastOpenAt: ago(day)}, 3, false},
		{"never mailed", engagementSignals{}, 3, false},
	} {
		if got := tc.e.sunset(tc.after); got != tc.want {
			t.Errorf("%s: sunset(%d) = %v, want %v", tc.name, tc.after, got, tc.want)
		}
	}
}

func lead(eng engagementSignals) dueNewLead {
	return dueNewLead{pair: ContactSequencePair{ContactID: uuid.New(), IsNewLead: true}, eng: eng}
}

func TestPickEngagedFirstRanksByTierThenScore(t *testing.T) {
	contactedNoEngagement := lead(engagementSignals{Sends: 4})
	neverContacted := lead(engagementSignals{})
	openedLongAgo := lead(engagementSignals{Sends: 2, LastOpenAt: ago(200 * day)})
	repliedRecently := lead(engagementSignals{Sends: 2, LastReplyAt: ago(3 * day)})
	clickedRecently := lead(engagementSignals{Sends: 2, LastClickAt: ago(3 * day)})

	// Listed worst first, so a list-order pick would be wrong.
	leads := []dueNewLead{contactedNoEngagement, neverContacted, openedLongAgo, clickedRecently, repliedRecently}
	got := pickEngagedFirst(leads, engNow)
	if got == nil || got.pair.ContactID != repliedRecently.pair.ContactID {
		t.Fatalf("the recent replier should go first, got %+v", got)
	}

	rest := []dueNewLead{contactedNoEngagement, neverContacted, openedLongAgo}
	if got := pickEngagedFirst(rest, engNow); got.pair.ContactID != openedLongAgo.pair.ContactID {
		t.Fatal("any engaged contact outranks never-contacted ones")
	}

	rest = []dueNewLead{contactedNoEngagement, neverContacted}
	if got := pickEngagedFirst(rest, engNow); got.pair.ContactID != neverContacted.pair.ContactID {
		t.Fatal("never-contacted outranks contacts who were mailed and never engaged")
	}
}

func TestPickEngagedFirstKeepsListOrderForTies(t *testing.T) {
	a, b := lead(engagementSignals{}), lead(engagementSignals{})
	if got := pickEngagedFirst([]dueNewLead{a, b}, engNow); got.pair.ContactID != a.pair.ContactID {
		t.Fatal("never-contacted leads should keep the campaign's contact order")
	}
	c, d := lead(engagementSignals{Sends: 3}), lead(engagementSignals{Sends: 9})
	if got := pickEngagedFirst([]dueNewLead{c, d}, engNow); got.pair.ContactID != c.pair.ContactID {
		t.Fatal("unengaged leads should keep the campaign's contact order")
	}
}

func TestPickEngagedFirstEmpty(t *testing.T) {
	if pickEngagedFirst(nil, engNow) != nil {
		t.Fatal("no due new leads means no pick")
	}
}

func TestNewLeadPolicyActive(t *testing.T) {
	if (NewLeadPolicy{}).active() {
		t.Fatal("the zero policy must leave the finder query unchanged")
	}
	if !(NewLeadPolicy{EngagedFirst: true}).active() || !(NewLeadPolicy{SunsetAfter: 3}).active() {
		t.Fatal("either rule activates the policy")
	}
}
