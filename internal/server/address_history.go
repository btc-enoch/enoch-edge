package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/enoch/enoch-edge/internal/address"
	"github.com/enoch/enoch-edge/internal/upstream"
)

// handleAddressHistory serves /v1/address_history/{addr}. Same multi-
// format input handling as /v1/utxos and /v1/balance — wallet may
// query by enoch1 or any segwit-v0 form, edge normalizes to the
// operator's enoch1-only API.
//
// Query params (?from=, ?limit=) pass through verbatim — operator
// owns the pagination semantics.
func handleAddressHistory(op *upstream.Client) http.HandlerFunc {
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

		path := "/address_history/" + enochAddr
		if raw := req.URL.RawQuery; raw != "" {
			path += "?" + raw
		}

		var got map[string]any
		status, err := op.GetJSON(req.Context(), path, &got)
		if err != nil {
			code := http.StatusBadGateway
			if status >= 400 && status < 500 {
				code = status
			}
			http.Error(w, "upstream /address_history: "+err.Error(), code)
			return
		}
		got["address"] = input

		w.Header().Set("Cache-Control", "public, max-age=2")
		_ = json.NewEncoder(w).Encode(got)
	}
}
