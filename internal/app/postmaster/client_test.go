package postmaster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

// fakeGoogle serves the token endpoint and the Postmaster API in the shape
// Google documents for both. The real API was not called to write these tests.
type fakeGoogle struct {
	srv *httptest.Server

	mu       sync.Mutex
	v1Calls  int
	v2Calls  int
	v1Query  map[string]string
	v2Body   map[string]any
	tokenHit int

	v1 func(w http.ResponseWriter, r *http.Request)
	v2 func(w http.ResponseWriter, r *http.Request)
	// token overrides the default access-token answer.
	token func(w http.ResponseWriter, r *http.Request)
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.tokenHit++
		f.mu.Unlock()
		if f.token != nil {
			f.token(w, r)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at","token_type":"Bearer","expires_in":3600}`)
	})
	mux.HandleFunc("/v1/domains/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.v1Calls++
		f.v1Query = map[string]string{}
		for k, v := range r.URL.Query() {
			f.v1Query[k] = v[0]
		}
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer at" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.v1(w, r)
	})
	mux.HandleFunc("/v2/domains/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.v2Calls++
		f.v2Body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&f.v2Body)
		f.mu.Unlock()
		if f.v2 == nil {
			http.Error(w, `{"error":{"code":403,"status":"PERMISSION_DENIED"}}`, http.StatusForbidden)
			return
		}
		f.v2(w, r)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGoogle) client(t *testing.T) *Client {
	t.Helper()
	creds := models.PostmasterCredentials{ClientID: "cid", ClientSecret: "secret", RefreshToken: "rt"}
	return newClient(context.Background(), creds, f.srv.URL, f.srv.URL+"/token", f.srv.Client())
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptrEq(p *float64, want float64) bool { return p != nil && *p == want }

func TestStatsV1ParsesDocumentedShapeAndPages(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = io.WriteString(w, `{
			  "trafficStats": [
			    {"name":"domains/example.com/trafficStats/20260101","userReportedSpamRatio":0.0004,
			     "domainReputation":"HIGH","spfSuccessRatio":1,"dkimSuccessRatio":0.99,"dmarcSuccessRatio":0.98,
			     "ipReputations":[{"reputation":"HIGH","ipCount":"2","sampleIps":["1.2.3.4"]}],
			     "deliveryErrors":[{"errorClass":"TEMPORARY_ERROR","errorType":"RATE_LIMIT_EXCEEDED","errorRatio":0.01}]},
			    {"name":"domains/example.com/trafficStats/20260102","domainReputation":"REPUTATION_CATEGORY_UNSPECIFIED"}
			  ],
			  "nextPageToken":"p2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"trafficStats":[
		    {"name":"domains/example.com/trafficStats/20260103","userReportedSpamRatio":1.7,"domainReputation":"bad"},
		    {"name":"not-a-name"},
		    {"userReportedSpamRatio":0.5}
		]}`)
	}

	got, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 days (bad names dropped), got %d: %+v", len(got), got)
	}

	d1 := got[0]
	if !d1.Date.Equal(day(2026, 1, 1)) || d1.DomainReputation != "high" ||
		!ptrEq(d1.SpamRate, 0.0004) || !ptrEq(d1.SPFRatio, 1) || !ptrEq(d1.DKIMRatio, 0.99) || !ptrEq(d1.DMARCRatio, 0.98) {
		t.Errorf("day 1 parsed wrong: %+v", d1)
	}
	var raw struct {
		DeliveryErrors []struct{ ErrorType string } `json:"deliveryErrors"`
	}
	if json.Unmarshal(d1.Raw, &raw) != nil || len(raw.DeliveryErrors) != 1 {
		t.Errorf("raw payload not kept: %s", d1.Raw)
	}

	// A low-volume day: Gmail names the day and sends nothing else.
	d2 := got[1]
	if d2.DomainReputation != "" || d2.SpamRate != nil || d2.SPFRatio != nil || d2.DKIMRatio != nil || d2.DMARCRatio != nil {
		t.Errorf("day with no data must keep every metric nil: %+v", d2)
	}

	// An impossible ratio is dropped rather than charted.
	d3 := got[2]
	if d3.DomainReputation != "bad" || d3.SpamRate != nil {
		t.Errorf("out-of-range ratio kept: %+v", d3)
	}

	if f.v1Calls != 2 || f.v2Calls != 0 {
		t.Errorf("v1 with data must not touch v2: v1=%d v2=%d", f.v1Calls, f.v2Calls)
	}
	// v1 treats the end date as exclusive, so the client asks for the day after.
	q := f.v1Query
	if q["startDate.year"] != "2026" || q["startDate.month"] != "1" || q["startDate.day"] != "1" ||
		q["endDate.day"] != "4" || q["pageToken"] != "p2" {
		t.Errorf("unexpected v1 query: %v", q)
	}
}

func TestStatsFallsBackToV2WhenV1HasNothing(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }
	f.v2 = func(w http.ResponseWriter, r *http.Request) {
		if f.v2Body["pageToken"] == nil {
			_, _ = io.WriteString(w, `{"domainStats":[
			  {"metric":"spam_rate","date":{"year":2026,"month":1,"day":2},"value":{"doubleValue":0.002}},
			  {"metric":"spf","date":{"year":2026,"month":1,"day":2},"value":{"floatValue":0.5}},
			  {"metric":"dkim","date":{"year":2026,"month":1,"day":2},"value":{"doubleValue":1}}
			 ],"nextPageToken":"next"}`)
			return
		}
		_, _ = io.WriteString(w, `{"domainStats":[
		  {"metric":"dmarc","date":{"year":2026,"month":1,"day":2},"value":{"doubleValue":0.9}},
		  {"metric":"spam_rate","date":{"year":2026,"month":1,"day":3},"value":{"doubleValue":0}},
		  {"metric":"tls_inbound","date":{"year":2026,"month":1,"day":3},"value":{"intValue":"1"}},
		  {"metric":"spf","date":{"year":2026,"month":1,"day":3},"value":{"stringValue":"n/a"}},
		  {"metric":"dkim","date":{"year":0,"month":0,"day":0},"value":{"doubleValue":1}}
		]}`)
	}

	got, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]models.PostmasterStat{}
	for _, s := range got {
		by[s.Date.Format("2006-01-02")] = s
	}
	if len(by) != 2 {
		t.Fatalf("want 2 days, got %+v", got)
	}
	d2 := by["2026-01-02"]
	if !ptrEq(d2.SpamRate, 0.002) || !ptrEq(d2.SPFRatio, 0.5) || !ptrEq(d2.DKIMRatio, 1) || !ptrEq(d2.DMARCRatio, 0.9) {
		t.Errorf("metrics from two pages were not folded into one day: %+v", d2)
	}
	if d2.DomainReputation != "" {
		t.Errorf("v2 has no reputation, got %q", d2.DomainReputation)
	}
	d3 := by["2026-01-03"]
	if !ptrEq(d3.SpamRate, 0) || d3.SPFRatio != nil || d3.DMARCRatio != nil {
		t.Errorf("a non-numeric value must stay nil and a zero must stay zero: %+v", d3)
	}

	body := f.v2Body
	if body["aggregationGranularity"] != "DAILY" {
		t.Errorf("must ask for daily rows: %v", body)
	}
	defs, _ := body["metricDefinitions"].([]any)
	if len(defs) != 8 {
		t.Errorf("want 8 metric definitions, got %d", len(defs))
	}
	first, _ := defs[0].(map[string]any)
	if first["name"] != "spam_rate" || first["baseMetric"].(map[string]any)["standardMetric"] != "SPAM_RATE" {
		t.Errorf("first metric wrong: %v", first)
	}
	rng := body["timeQuery"].(map[string]any)["dateRanges"].(map[string]any)["dateRanges"].([]any)[0].(map[string]any)
	if rng["end"].(map[string]any)["day"] != float64(3) {
		t.Errorf("v2 end date is inclusive and must be the last day asked for: %v", rng)
	}
}

func TestStatsUsesV2WhenV1IsGone(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
	}
	f.v2 = func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"domainStats":[{"metric":"spam_rate","date":{"year":2026,"month":1,"day":2},"value":{"doubleValue":0.001}}]}`)
	}
	got, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if err != nil || len(got) != 1 || !ptrEq(got[0].SpamRate, 0.001) {
		t.Fatalf("v1 retired must fall through to v2: %+v %v", got, err)
	}
}

func TestStatsDomainNotRegistered(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":403,"message":"User does not have permission","status":"PERMISSION_DENIED"}}`, http.StatusForbidden)
	}
	_, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if !errors.Is(err, ErrNoAccess) {
		t.Fatalf("an unverified domain is ErrNoAccess, got %v", err)
	}
}

func TestStatsNoDataIsNotAnError(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }
	got, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if err != nil || len(got) != 0 {
		t.Fatalf("a low-volume domain has no rows and no error: %+v %v", got, err)
	}
}

func TestStatsAccountLevelRefusals(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"unauthorized":       {http.StatusUnauthorized, `{"error":{"code":401,"message":"Invalid Credentials"}}`},
		"insufficient scope": {http.StatusForbidden, `{"error":{"code":403,"message":"Request had insufficient authentication scopes.","details":[{"reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}]}}`},
		"api not enabled":    {http.StatusForbidden, `{"error":{"code":403,"message":"API has not been used in project","details":[{"reason":"SERVICE_DISABLED"}]}}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeGoogle(t)
			f.v1 = func(w http.ResponseWriter, r *http.Request) { http.Error(w, c.body, c.status) }
			f.v2 = func(w http.ResponseWriter, r *http.Request) { http.Error(w, c.body, c.status) }
			_, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
			if !errors.Is(err, ErrAccount) {
				t.Fatalf("want ErrAccount, got %v", err)
			}
		})
	}
}

func TestStatsServerErrorIsTransient(t *testing.T) {
	f := newFakeGoogle(t)
	f.v1 = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"backend error"}}`, http.StatusBadGateway)
	}
	f.v2 = f.v1
	_, err := f.client(t).Stats(context.Background(), "example.com", day(2026, 1, 1), day(2026, 1, 3))
	if err == nil || errors.Is(err, ErrAccount) || errors.Is(err, ErrNoAccess) {
		t.Fatalf("a 502 is neither an account nor an access verdict: %v", err)
	}
}

func TestCheckRejectsRevokedRefreshToken(t *testing.T) {
	f := newFakeGoogle(t)
	f.token = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
	}
	err := f.client(t).Check()
	if !errors.Is(err, ErrAccount) {
		t.Fatalf("want ErrAccount, got %v", err)
	}
	if err := f.client(t).Check(); err == nil {
		t.Fatal("second check should still fail")
	}
}

func TestCheckAcceptsWorkingCredentials(t *testing.T) {
	f := newFakeGoogle(t)
	if err := f.client(t).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestTokenEndpointOutageIsNotAnAccountError(t *testing.T) {
	f := newFakeGoogle(t)
	f.token = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }
	err := f.client(t).Check()
	if err == nil || errors.Is(err, ErrAccount) {
		t.Fatalf("a 500 from the token endpoint must stay transient: %v", err)
	}
}
