// Package upstream is the edge's HTTP client for the operator(s).
// Multi-operator fan-out (read-quorum, agreement checks) gets layered
// on top of this Client later — for now, a single operator behind a
// single base URL is enough for the wallet PoC.
package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

// GetJSON hits a path on the upstream operator and decodes the body
// into out. The HTTP status is returned separately so callers can
// distinguish 4xx (proxy through to the wallet) from 5xx / network
// failures (return 502 from edge).
func (c *Client) GetJSON(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upstream %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("upstream %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode %s: %w", path, err)
	}
	return resp.StatusCode, nil
}

// PostJSON sends a POST with a JSON body and returns the upstream
// status and response body. Unlike GetJSON, 4xx/5xx is NOT a Go
// error — the body carries the operator's own error message that
// the wallet needs to see verbatim (e.g., "insufficient funds",
// "script verification failed"). err is reserved for network-level
// or protocol-level failures, where edge should synthesize a 502.
func (c *Client) PostJSON(ctx context.Context, path string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("upstream %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read upstream %s: %w", path, err)
	}
	return resp.StatusCode, respBody, nil
}
