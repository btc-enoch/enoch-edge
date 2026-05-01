package server

import (
	"encoding/json"
	"net/http"

	"github.com/enoch/enoch-edge/internal/upstream"
)

// handlePendingWithdrawals — thin proxy of the operator's
// /pending_withdrawals. Returns every withdrawal that hasn't yet
// been broadcast to L1 (queued + post-round-1 awaiting challenge
// window). Once a withdrawal is broadcast it leaves this list;
// wallets correlate against /v1/address_history burn rows to
// surface broadcast/completed states.
//
// Filtering is currently global (no per-address scoping) — same as
// the underlying operator endpoint. Privacy work will revisit.
func handlePendingWithdrawals(op *upstream.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var got map[string]any
		status, err := op.GetJSON(req.Context(), "/pending_withdrawals", &got)
		if err != nil {
			code := http.StatusBadGateway
			if status >= 400 && status < 500 {
				code = status
			}
			http.Error(w, "upstream /pending_withdrawals: "+err.Error(), code)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=2")
		_ = json.NewEncoder(w).Encode(got)
	}
}
