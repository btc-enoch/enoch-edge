package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/enoch/enoch-edge/internal/address"
	"github.com/enoch/enoch-edge/internal/upstream"
)

// handleBalance serves /v1/balance/{addr}. Same address-format
// normalization as /v1/utxos — edge does the bridging so wallets can
// query by either Enoch- or Bitcoin-encoded form.
func handleBalance(op *upstream.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		input := chi.URLParam(req, "addr")
		pkh, err := address.DecodeToPKH(input)
		if err != nil {
			http.Error(w, "decode address: "+err.Error(), http.StatusBadRequest)
			return
		}
		enochAddr, err := address.EncodeEnoch(pkh)
		if err != nil {
			http.Error(w, "encode enoch: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var got map[string]any
		status, err := op.GetJSON(req.Context(), "/balance/"+enochAddr, &got)
		if err != nil {
			code := http.StatusBadGateway
			if status >= 400 && status < 500 {
				code = status
			}
			http.Error(w, "upstream /balance: "+err.Error(), code)
			return
		}
		got["address"] = input

		w.Header().Set("Cache-Control", "public, max-age=2")
		_ = json.NewEncoder(w).Encode(got)
	}
}
