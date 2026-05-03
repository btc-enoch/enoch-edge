package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FederationPool wraps N upstream Clients indexed by operator id.
// /submit_tx routes to the current-leader client; on 503 with a
// leader_id hint, the pool refreshes its leader pointer and retries
// once. Other endpoints stay simple: callers grab Pool.Leader() (or
// any client) and use it like a single-operator Client.
//
// The pool does not poll /info on its own — it only learns about
// leader changes from the 503 redirects produced by non-leader
// operators when /submit_tx lands on them. That matches the spec's
// fail-then-redirect model (slice 4 lock-in, Q3) without adding a
// background poller.
type FederationPool struct {
	clients []*Client

	mu     sync.RWMutex
	leader int // index into clients; updated on 503 redirects
}

// NewFederationPool builds a pool from a slice of base URLs ordered
// by operator id. timeout is applied to every per-request HTTP call.
func NewFederationPool(urls []string, timeout time.Duration) (*FederationPool, error) {
	if len(urls) == 0 {
		return nil, errors.New("federation pool requires at least one URL")
	}
	clients := make([]*Client, len(urls))
	for i, u := range urls {
		clients[i] = New(u, timeout)
	}
	return &FederationPool{clients: clients}, nil
}

// Leader returns the client for the operator the pool currently
// believes is leading height-N+1. Used for non-/submit_tx reads where
// edge wants to talk to a "live" operator without bothering with
// retry semantics.
func (p *FederationPool) Leader() *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients[p.leader]
}

// LeaderIndex is for tests + log lines.
func (p *FederationPool) LeaderIndex() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.leader
}

// Size returns the number of operators in the pool.
func (p *FederationPool) Size() int { return len(p.clients) }

// SubmitTx posts to /submit_tx on the current-leader client. If the
// response is 503 with a `leader_id` field in the body, it switches
// the pool's leader pointer and retries once. Other status codes
// (200, 4xx, 5xx without redirect, network errors) are returned as-is.
//
// Returns the upstream's HTTP status + body verbatim so the
// wallet-facing handler can pass them through unchanged.
func (p *FederationPool) SubmitTx(ctx context.Context, body []byte) (int, []byte, error) {
	p.mu.RLock()
	cur := p.leader
	p.mu.RUnlock()

	status, resp, err := p.clients[cur].PostJSON(ctx, "/submit_tx", body)
	if err != nil {
		return status, resp, err
	}
	if status != http.StatusServiceUnavailable {
		return status, resp, nil
	}

	// 503 — try to read a leader_id redirect. If absent or invalid,
	// pass through the 503 as-is.
	var redirect struct {
		LeaderID *int   `json:"leader_id"`
		Error    string `json:"error"`
	}
	if jsonErr := json.Unmarshal(resp, &redirect); jsonErr != nil || redirect.LeaderID == nil {
		return status, resp, nil
	}
	target := *redirect.LeaderID
	if target < 0 || target >= len(p.clients) || target == cur {
		return status, resp, nil
	}

	p.mu.Lock()
	p.leader = target
	p.mu.Unlock()

	return p.clients[target].PostJSON(ctx, "/submit_tx", body)
}

// String is for log lines.
func (p *FederationPool) String() string {
	urls := make([]string, len(p.clients))
	for i, c := range p.clients {
		urls[i] = c.baseURL
	}
	return fmt.Sprintf("federation[%d operators leader=%d urls=%s]",
		len(urls), p.LeaderIndex(), strings.Join(urls, ","))
}
