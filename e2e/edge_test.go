//go:build e2e

// Package e2e runs end-to-end tests against a live edge process and
// (transitively) a live operator. Skipped from `go test ./...` —
// run with `go test -tags=e2e ./e2e/...` after `make up` in both
// enoch and enoch-edge.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/enoch/enoch-edge/internal/address"
)

// emptyEnochAddr returns a known-empty enoch1 address (all-zero pkh)
// computed at test time, so the bech32 checksum is guaranteed valid
// regardless of any future HRP or library tweaks.
func emptyEnochAddr(t *testing.T) string {
	t.Helper()
	zeros := make([]byte, 20)
	addr, err := address.EncodeEnoch(zeros)
	if err != nil {
		t.Fatalf("EncodeEnoch: %v", err)
	}
	return addr
}

func edgeURL() string {
	if v := os.Getenv("EDGE_URL"); v != "" {
		return v
	}
	return "http://localhost:8081"
}

func TestHealth(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

// TestBalance_UnknownEnochAddress hits /v1/balance/{addr} with a
// well-formed enoch1 address that has no UTXOs. We expect a 200
// with balance_satoshi=0, utxo_count=0 — proves the address pipe
// (decode → encode → operator → echo input) works for an empty
// account, without depending on a funded fixture.
func TestBalance_UnknownEnochAddress(t *testing.T) {
	addr := emptyEnochAddr(t)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/balance/" + addr)
	if err != nil {
		t.Fatalf("GET /v1/balance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Address        string `json:"address"`
		BalanceSatoshi uint64 `json:"balance_satoshi"`
		UTXOCount      int    `json:"utxo_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Address != addr {
		t.Errorf("address = %q, want %q (edge should echo wallet's input form)", body.Address, addr)
	}
	if body.BalanceSatoshi != 0 || body.UTXOCount != 0 {
		t.Errorf("balance = %d / count = %d, want both zero", body.BalanceSatoshi, body.UTXOCount)
	}
}

// TestUTXOs_EmptyAddress verifies the /v1/utxos/{addr} pipe end-to-
// end against an unfunded enoch1. Should return 200 with an empty
// utxos array, not 404 — wallets need to distinguish "no balance"
// from "broken endpoint".
func TestUTXOs_EmptyAddress(t *testing.T) {
	addr := emptyEnochAddr(t)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/utxos/" + addr)
	if err != nil {
		t.Fatalf("GET /v1/utxos: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Address string `json:"address"`
		UTXOs   []any  `json:"utxos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Address != addr {
		t.Errorf("address = %q, want %q", body.Address, addr)
	}
	if len(body.UTXOs) != 0 {
		t.Errorf("utxos = %v, want empty", body.UTXOs)
	}
}

// TestAddressHistory_EmptyAddress verifies the index pipe end-to-end
// against an unfunded enoch1. Should return 200 with entries:[] —
// proves the operator wrote no spurious entries for an unrelated pkh
// and edge passes the empty list through.
func TestAddressHistory_EmptyAddress(t *testing.T) {
	addr := emptyEnochAddr(t)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/address_history/" + addr)
	if err != nil {
		t.Fatalf("GET /v1/address_history: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Address string `json:"address"`
		Entries []any  `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Address != addr {
		t.Errorf("address = %q, want %q", body.Address, addr)
	}
	if len(body.Entries) != 0 {
		t.Errorf("entries = %v, want empty", body.Entries)
	}
}

// TestFeeOracle hits the live oracle (mempool.space by default) and
// asserts the response shape. We don't pin specific values — fee
// rates are real-world data — but we do require the rate fields to
// be present and non-negative, which proves the upstream→Snapshot
// translation works.
//
// Skipped if no internet (FEE_ORACLE_URL pointing at the default
// public host but the test env is offline). The skip avoids a flaky
// CI signal; production-style tests against an offline stub would
// live in unit tests, not here.
func TestFeeOracle(t *testing.T) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/fee_oracle")
	if err != nil {
		t.Fatalf("GET /v1/fee_oracle: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadGateway {
		t.Skipf("fee oracle upstream unreachable (502) — likely offline test env")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Source string `json:"source"`
		AsOf   string `json:"as_of"`
		Rates  struct {
			Fastest  uint64 `json:"fastest"`
			HalfHour uint64 `json:"half_hour"`
			Hour     uint64 `json:"hour"`
			Economy  uint64 `json:"economy"`
			Minimum  uint64 `json:"minimum"`
		} `json:"rates_sat_per_vb"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Source == "" {
		t.Error("source empty")
	}
	if body.AsOf == "" {
		t.Error("as_of empty")
	}
	// Fastest >= Minimum is a structural sanity check that doesn't
	// depend on the day's actual fee market.
	if body.Rates.Fastest < body.Rates.Minimum {
		t.Errorf("fastest (%d) < minimum (%d) — implausible", body.Rates.Fastest, body.Rates.Minimum)
	}
}

// TestEvents_ConnectionEstablishes verifies the SSE pipe:
// edge → upstream operator/events → bus → wallet handler. We don't
// trigger a real tx here (that needs the wallet container with
// signing keys); we just confirm the wallet's HTTP/1.1 stream opens
// with the right SSE headers and the initial ": connected" comment
// arrives. Full event-emit verification is the manual curl + wallet
// pay path, since pulling that into the test container is heavy.
func TestEvents_ConnectionEstablishes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, edgeURL()+"/v1/events", nil)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "connected") {
			return // success
		}
	}
	t.Fatalf("never received `: connected` (scanner err: %v)", scanner.Err())
}

// TestEvents_BadAddressFilter rejects malformed ?addr= before opening
// the stream. Same rationale as the read endpoints: input validation
// errors are JSON 400s, not half-open streams the wallet has to debug.
func TestEvents_BadAddressFilter(t *testing.T) {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/events?addr=not-an-address")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestSubmitTx_MalformedBody verifies the proxy hop on the write
// path: edge forwards the POST body to the operator, and the
// operator's 400 (invalid JSON) comes back to the wallet untouched.
// Submitting a real signed tx needs a funded UTXO + key material —
// out of scope for this layer's e2e.
func TestSubmitTx_MalformedBody(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Post(edgeURL()+"/v1/submit_tx", "application/json", bytes.NewReader([]byte(`not json`)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestBalance_BadAddress confirms edge surfaces a 400 (not 502) when
// the client sends a malformed address — the operator never gets
// hit, so this is purely the decoder doing its job.
func TestBalance_BadAddress(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/balance/not-an-address")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestInfo(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(edgeURL() + "/v1/info")
	if err != nil {
		t.Fatalf("GET /v1/info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (is the operator running?)", resp.StatusCode)
	}
	var body struct {
		Edge struct {
			Version string `json:"version"`
		} `json:"edge"`
		Operator map[string]any `json:"operator"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Edge.Version == "" {
		t.Error("edge.version empty")
	}
	if body.Operator == nil {
		t.Error("operator missing")
	}
}
