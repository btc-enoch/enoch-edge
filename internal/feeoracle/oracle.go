// Package feeoracle fetches Bitcoin L1 fee-rate estimates from a
// public source (default: mempool.space) and caches them so a fleet
// of wallets hitting /v1/fee_oracle doesn't hammer the upstream.
//
// Fee rates are needed by wallets to size peg-out (withdrawal-to-L1)
// transactions. The operator does not itself care about L1 fees —
// this is purely a wallet-UX concern, so it lives at the edge.
package feeoracle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Rates is the wire shape edge returns to wallets. Keys are in
// snake_case for consistency with the rest of the wallet API.
// Values are sat/vbyte.
type Rates struct {
	Fastest  uint64 `json:"fastest"`
	HalfHour uint64 `json:"half_hour"`
	Hour     uint64 `json:"hour"`
	Economy  uint64 `json:"economy"`
	Minimum  uint64 `json:"minimum"`
}

// Snapshot is one cached read from upstream. AsOf is the time the
// fetch completed (not the time wall-clock requested it), so wallets
// can tell whether the oracle is stale.
type Snapshot struct {
	Source string    `json:"source"`
	AsOf   time.Time `json:"as_of"`
	Rates  Rates     `json:"rates_sat_per_vb"`
}

// Client is a single-flight cache around a fee-rate API. Concurrent
// callers within the TTL share the cached snapshot; concurrent
// callers across an expiry serialize on the mutex (a brief lock
// is fine for this read-heavy / refetch-rare workload).
type Client struct {
	upstream string
	source   string // human-readable label for the snapshot
	httpC    *http.Client
	ttl      time.Duration

	mu     sync.Mutex
	cached *Snapshot
}

// New builds a Client. ttl is how long a snapshot is served before
// the next call refetches.
func New(upstream string, ttl time.Duration) *Client {
	return &Client{
		upstream: upstream,
		source:   sourceLabel(upstream),
		httpC:    &http.Client{Timeout: 5 * time.Second},
		ttl:      ttl,
	}
}

// Get returns a fresh-or-cached Snapshot. On upstream failure with
// a previously-good snapshot in cache, returns the stale snapshot
// rather than failing the wallet — peg-out estimation tolerates
// staleness better than it tolerates a hard error.
func (c *Client) Get(ctx context.Context) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.cached.AsOf) < c.ttl {
		return c.cached, nil
	}

	snap, err := c.fetch(ctx)
	if err != nil {
		if c.cached != nil {
			return c.cached, nil
		}
		return nil, err
	}
	c.cached = snap
	return snap, nil
}

// mempoolResponse mirrors mempool.space's /api/v1/fees/recommended.
// Field names are theirs (camelCase); we translate to snake_case in
// the public Rates type.
type mempoolResponse struct {
	FastestFee  uint64 `json:"fastestFee"`
	HalfHourFee uint64 `json:"halfHourFee"`
	HourFee     uint64 `json:"hourFee"`
	EconomyFee  uint64 `json:"economyFee"`
	MinimumFee  uint64 `json:"minimumFee"`
}

func (c *Client) fetch(ctx context.Context) (*Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upstream, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fee oracle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fee oracle: status %d", resp.StatusCode)
	}
	var raw mempoolResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("fee oracle decode: %w", err)
	}
	return &Snapshot{
		Source: c.source,
		AsOf:   time.Now().UTC(),
		Rates: Rates{
			Fastest:  raw.FastestFee,
			HalfHour: raw.HalfHourFee,
			Hour:     raw.HourFee,
			Economy:  raw.EconomyFee,
			Minimum:  raw.MinimumFee,
		},
	}, nil
}

// sourceLabel returns a friendly tag for the snapshot's "source"
// field. Avoids putting full upstream URLs in wallet-facing JSON.
func sourceLabel(upstream string) string {
	switch {
	case contains(upstream, "mempool.space"):
		return "mempool.space"
	case contains(upstream, "blockstream"):
		return "blockstream.info"
	default:
		return "custom"
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
