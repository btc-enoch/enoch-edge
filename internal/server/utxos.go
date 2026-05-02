package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/enoch/enoch-edge/internal/address"
	"github.com/enoch/enoch-edge/internal/upstream"
)

// handleUTXOs serves /v1/utxos/{addr}. Wallet may pass any of
// enoch1.../bc1q.../tb1q.../bcrt1q...; edge normalizes to the
// operator's enoch1-only API and echoes the wallet's input form
// back in the response so the client doesn't have to track which
// encoding it sent.
func handleUTXOs(op *upstream.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		input := chi.URLParam(req, "addr")
		enochAddr, err := address.NormalizeToEnoch(input)
		if err != nil {
			http.Error(w, "decode address: "+err.Error(), http.StatusBadRequest)
			return
		}

		var got map[string]any
		status, err := op.GetJSON(req.Context(), "/utxos/"+enochAddr, &got)
		if err != nil {
			code := http.StatusBadGateway
			if status >= 400 && status < 500 {
				code = status
			}
			http.Error(w, "upstream /utxos: "+err.Error(), code)
			return
		}
		// Replace the operator's echoed address (always enoch1) with
		// whatever the wallet sent — friendlier UX for clients that
		// query by their bc1q/bcrt1q form.
		got["address"] = input

		w.Header().Set("Cache-Control", "public, max-age=2")
		_ = json.NewEncoder(w).Encode(got)
	}
}
