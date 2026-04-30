package server

import (
	"encoding/json"
	"net/http"

	"github.com/enoch/enoch-edge/internal/feeoracle"
)

// handleFeeOracle serves /v1/fee_oracle. Wallet calls this when
// constructing a peg-out (withdrawal-to-L1) transaction so the user
// can see the current fee market and pick a speed/cost tradeoff.
//
// Cache-Control matches the in-process cache TTL — clients can
// safely poll every few seconds without thrashing the upstream.
func handleFeeOracle(oracle *feeoracle.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		snap, err := oracle.Get(req.Context())
		if err != nil {
			http.Error(w, "fee oracle: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=30")
		_ = json.NewEncoder(w).Encode(snap)
	}
}
