package salesblink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/errx"
)

// client is the outbound-only HTTP client to SalesBlink's public API.
//
// Auth is a bare API key in the Authorization header — SalesBlink does not use
// a "Bearer " prefix, so do not add one.
type client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newClient(baseURL, apiKey string) *client {
	return &client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// remoteError covers the error shapes SalesBlink returns.
type remoteError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (c *client) do(ctx context.Context, method, path string, body any, out any) *errx.Error {
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return errx.InternalError()
		}
		buf = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, buf)
	if err != nil {
		return errx.InternalError()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dakiya-salesblink/1.0")
	if c.apiKey != "" {
		req.Header.Set("Authorization", c.apiKey)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return errx.NewWithIdentifier(errx.ServiceUnavailable, "salesblink_unreachable",
			fmt.Sprintf("SalesBlink could not be reached: %v", err))
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))

	if res.StatusCode >= 400 {
		var re remoteError
		_ = json.Unmarshal(raw, &re)
		msg := re.Message
		if msg == "" {
			msg = re.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("SalesBlink answered %d", res.StatusCode)
		}
		// 401/403 almost always means the key is wrong or revoked; say so
		// rather than surfacing SalesBlink's generic text.
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return errx.NewWithIdentifier(errx.Unauthorized, "salesblink_bad_key",
				"SalesBlink rejected the API key. Check it in Settings > SalesBlink.")
		}
		return errx.NewWithIdentifier(errx.Code(res.StatusCode), "salesblink_remote", msg)
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return errx.NewWithIdentifier(errx.ServiceUnavailable, "salesblink_bad_response",
				"SalesBlink returned an unreadable response.")
		}
	}
	return nil
}
