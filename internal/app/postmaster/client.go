// Package postmaster pulls Gmail's own view of each sending domain from the
// Gmail Postmaster Tools API and serves it back per workspace.
package postmaster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/bitan-del/dakiya-mail/internal/models"
)

const (
	apiBase  = "https://gmailpostmastertools.googleapis.com"
	tokenURL = "https://oauth2.googleapis.com/token"

	pageSize = 200
	maxPages = 20
)

var (
	// ErrNoAccess means the domain is not registered or not verified for the connected Google account.
	ErrNoAccess = errors.New("postmaster: domain not available to this Google account")
	// ErrAccount means the credentials themselves were refused: revoked token, missing scope, API not enabled.
	ErrAccount = errors.New("postmaster: google refused the credentials")
)

// Client talks to the Postmaster Tools API as one Google account.
type Client struct {
	hc   *http.Client
	ts   oauth2.TokenSource
	base string
}

// NewClient authenticates with the stored refresh token. The context must
// outlive every call made through the client: it also serves token refreshes.
func NewClient(ctx context.Context, c models.PostmasterCredentials) *Client {
	return newClient(ctx, c, apiBase, tokenURL, nil)
}

func newClient(ctx context.Context, c models.PostmasterCredentials, base, tokenEndpoint string, hc *http.Client) *Client {
	if hc != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}
	cfg := &oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: oauth2.Endpoint{TokenURL: tokenEndpoint, AuthStyle: oauth2.AuthStyleInParams}}
	ts := oauth2.ReuseTokenSource(nil, cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: c.RefreshToken}))
	out := oauth2.NewClient(ctx, ts)
	out.Timeout = 30 * time.Second
	return &Client{hc: out, ts: ts, base: strings.TrimRight(base, "/")}
}

// Check proves the client id, secret and refresh token work by minting an access token.
func (c *Client) Check() error {
	_, err := c.ts.Token()
	return classify(err)
}

// Stats returns the daily stats of one domain for from..to inclusive. It asks
// the v1 API first because only v1 carries domain reputation, then falls back
// to v2 when v1 has nothing: Google is retiring v1 and v2 has no reputation.
func (c *Client) Stats(ctx context.Context, domain string, from, to time.Time) ([]models.PostmasterStat, error) {
	v1, v1err := c.trafficStatsV1(ctx, domain, from, to)
	if v1err == nil && len(v1) > 0 {
		return v1, nil
	}
	v2, v2err := c.domainStatsV2(ctx, domain, from, to)
	switch {
	case v2err == nil:
		return v2, nil
	case v1err == nil:
		return nil, nil
	case errors.Is(v1err, ErrNoAccess) && errors.Is(v2err, ErrNoAccess):
		return nil, ErrNoAccess
	case !errors.Is(v1err, ErrNoAccess):
		return nil, v1err
	}
	return nil, v2err
}

func (c *Client) trafficStatsV1(ctx context.Context, domain string, from, to time.Time) ([]models.PostmasterStat, error) {
	q := url.Values{"pageSize": {strconv.Itoa(pageSize)}}
	setDate(q, "startDate", from)
	setDate(q, "endDate", to.AddDate(0, 0, 1)) // v1 treats the end as exclusive

	var out []models.PostmasterStat
	for range maxPages {
		var resp struct {
			TrafficStats  []json.RawMessage `json:"trafficStats"`
			NextPageToken string            `json:"nextPageToken"`
		}
		u := fmt.Sprintf("%s/v1/domains/%s/trafficStats?%s", c.base, url.PathEscape(domain), q.Encode())
		if err := c.do(ctx, http.MethodGet, u, nil, &resp); err != nil {
			return nil, err
		}
		for _, raw := range resp.TrafficStats {
			if st, ok := parseTrafficStats(raw); ok {
				out = append(out, st)
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		q.Set("pageToken", resp.NextPageToken)
	}
	return out, nil
}

// parseTrafficStats reads one v1 TrafficStats object. A day whose name does not
// parse is dropped; every metric Gmail omitted stays nil.
func parseTrafficStats(raw json.RawMessage) (models.PostmasterStat, bool) {
	var t struct {
		Name                  string   `json:"name"`
		DomainReputation      string   `json:"domainReputation"`
		UserReportedSpamRatio *float64 `json:"userReportedSpamRatio"`
		SPF                   *float64 `json:"spfSuccessRatio"`
		DKIM                  *float64 `json:"dkimSuccessRatio"`
		DMARC                 *float64 `json:"dmarcSuccessRatio"`
	}
	if json.Unmarshal(raw, &t) != nil {
		return models.PostmasterStat{}, false
	}
	// domains/{domain}/trafficStats/{yyyymmdd}
	day, err := time.Parse("20060102", t.Name[strings.LastIndex(t.Name, "/")+1:])
	if err != nil {
		return models.PostmasterStat{}, false
	}
	return models.PostmasterStat{
		Date:             day,
		DomainReputation: reputation(t.DomainReputation),
		SpamRate:         ratio(t.UserReportedSpamRatio),
		SPFRatio:         ratio(t.SPF),
		DKIMRatio:        ratio(t.DKIM),
		DMARCRatio:       ratio(t.DMARC),
		Raw:              raw,
	}, true
}

// v2 metric names, echoed back by Google in each DomainStat.metric.
const (
	mSpam    = "spam_rate"
	mSPF     = "spf"
	mDKIM    = "dkim"
	mDMARC   = "dmarc"
	mTLSIn   = "tls_inbound"
	mTLSOut  = "tls_outbound"
	mReject  = "delivery_reject"
	mTempErr = "delivery_temp_fail"
)

type metricDef struct {
	Name       string            `json:"name"`
	BaseMetric map[string]string `json:"baseMetric"`
	Filter     string            `json:"filter,omitempty"`
}

func metric(name, standard, filter string) metricDef {
	return metricDef{Name: name, BaseMetric: map[string]string{"standardMetric": standard}, Filter: filter}
}

func (c *Client) domainStatsV2(ctx context.Context, domain string, from, to time.Time) ([]models.PostmasterStat, error) {
	body := map[string]any{
		"metricDefinitions": []metricDef{
			metric(mSpam, "SPAM_RATE", ""),
			metric(mSPF, "AUTH_SUCCESS_RATE", `auth_type = "spf"`),
			metric(mDKIM, "AUTH_SUCCESS_RATE", `auth_type = "dkim"`),
			metric(mDMARC, "AUTH_SUCCESS_RATE", `auth_type = "dmarc"`),
			metric(mTLSIn, "TLS_ENCRYPTION_RATE", `traffic_direction = "inbound"`),
			metric(mTLSOut, "TLS_ENCRYPTION_RATE", `traffic_direction = "outbound"`),
			metric(mReject, "DELIVERY_ERROR_RATE", `error_type = "reject"`),
			metric(mTempErr, "DELIVERY_ERROR_RATE", `error_type = "temp_fail"`),
		},
		"timeQuery": map[string]any{"dateRanges": map[string]any{"dateRanges": []map[string]any{
			{"start": dateObj(from), "end": dateObj(to)}, // v2 ends are inclusive
		}}},
		"aggregationGranularity": "DAILY",
		"pageSize":               pageSize,
	}

	var rows []v2Stat
	for range maxPages {
		var resp struct {
			DomainStats   []v2Stat `json:"domainStats"`
			NextPageToken string   `json:"nextPageToken"`
		}
		u := fmt.Sprintf("%s/v2/domains/%s/domainStats:query", c.base, url.PathEscape(domain))
		if err := c.do(ctx, http.MethodPost, u, body, &resp); err != nil {
			return nil, err
		}
		rows = append(rows, resp.DomainStats...)
		if resp.NextPageToken == "" {
			break
		}
		body["pageToken"] = resp.NextPageToken
	}
	return pivotV2(rows), nil
}

// v2Stat is one DomainStat: a single metric value on a single day.
type v2Stat struct {
	Metric string `json:"metric"`
	Date   struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"date"`
	Value struct {
		Double *float64 `json:"doubleValue"`
		Float  *float64 `json:"floatValue"`
		Int    string   `json:"intValue"`
	} `json:"value"`
}

func (s v2Stat) number() (float64, bool) {
	switch {
	case s.Value.Double != nil:
		return *s.Value.Double, true
	case s.Value.Float != nil:
		return *s.Value.Float, true
	}
	n, err := strconv.ParseFloat(s.Value.Int, 64)
	return n, err == nil
}

// pivotV2 folds metric-per-row v2 output into one stat per day. v2 carries no
// reputation, so that stays empty; the other metrics ride along in raw.
func pivotV2(rows []v2Stat) []models.PostmasterStat {
	byDay := map[time.Time]map[string]float64{}
	for _, r := range rows {
		n, ok := r.number()
		if !ok || r.Date.Year == 0 || r.Date.Month == 0 || r.Date.Day == 0 {
			continue
		}
		day := time.Date(r.Date.Year, time.Month(r.Date.Month), r.Date.Day, 0, 0, 0, 0, time.UTC)
		if byDay[day] == nil {
			byDay[day] = map[string]float64{}
		}
		byDay[day][r.Metric] = n
	}

	out := make([]models.PostmasterStat, 0, len(byDay))
	for day, m := range byDay {
		get := func(k string) *float64 {
			if v, ok := m[k]; ok {
				return ratio(&v)
			}
			return nil
		}
		raw, _ := json.Marshal(map[string]any{"source": "v2", "metrics": m})
		out = append(out, models.PostmasterStat{
			Date:       day,
			SpamRate:   get(mSpam),
			SPFRatio:   get(mSPF),
			DKIMRatio:  get(mDKIM),
			DMARCRatio: get(mDMARC),
			Raw:        raw,
		})
	}
	return out
}

func (c *Client) do(ctx context.Context, method, u string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return classify(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return apiError(resp.StatusCode, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// classify turns a token-endpoint refusal into ErrAccount; everything else is transient.
func classify(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.Response != nil && (re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized) {
		return fmt.Errorf("%w: %s", ErrAccount, cut(strings.TrimSpace(re.ErrorCode+" "+re.ErrorDescription), 200))
	}
	return err
}

func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	msg := cut(e.Error.Message, 200)
	if msg == "" {
		msg = http.StatusText(status)
	}
	text := string(body)
	switch {
	case status == http.StatusUnauthorized,
		status == http.StatusForbidden && (strings.Contains(text, "ACCESS_TOKEN_SCOPE_INSUFFICIENT") || strings.Contains(text, "SERVICE_DISABLED")):
		return fmt.Errorf("%w: %s", ErrAccount, msg)
	case status == http.StatusForbidden, status == http.StatusNotFound:
		return ErrNoAccess
	}
	return fmt.Errorf("postmaster: google returned %d: %s", status, msg)
}

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func dateObj(t time.Time) map[string]int {
	return map[string]int{"year": t.Year(), "month": int(t.Month()), "day": t.Day()}
}

// setDate writes a google.type.Date query parameter as prefix.year/month/day.
func setDate(q url.Values, prefix string, t time.Time) {
	q.Set(prefix+".year", strconv.Itoa(t.Year()))
	q.Set(prefix+".month", strconv.Itoa(int(t.Month())))
	q.Set(prefix+".day", strconv.Itoa(t.Day()))
}

// ratio keeps only a real 0..1 value; anything else is treated as not reported.
func ratio(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || *v < 0 || *v > 1 {
		return nil
	}
	return v
}

func reputation(s string) string {
	switch r := strings.ToLower(strings.TrimSpace(s)); r {
	case "high", "medium", "low", "bad":
		return r
	}
	return ""
}
