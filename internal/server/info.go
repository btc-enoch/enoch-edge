package server

import (
	"encoding/json"
	"net/http"

	"github.com/enoch/enoch-edge/internal/upstream"
)

const (
	edgeVersion         = "0.1.0"
	edgeProtocolVersion = 1
)

// infoResponse wraps the operator's /info verbatim under "operator"
// so edge doesn't need to keep a typed schema in lockstep with
// operator-side field additions.
type infoResponse struct {
	Edge     edgeMetadata   `json:"edge"`
	Operator map[string]any `json:"operator"`
}

type edgeMetadata struct {
	Version         string `json:"version"`
	ProtocolVersion uint32 `json:"protocol_version"`
}

// handleInfo proxies the operator's /info response and wraps it
// with edge-side metadata. Wallets call this once on startup to
// learn the operator's pubkey, addresses, and fee schedule, plus
// the edge version they're talking to.
func handleInfo(op *upstream.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var got map[string]any
		status, err := op.GetJSON(req.Context(), "/info", &got)
		if err != nil {
			code := http.StatusBadGateway
			if status >= 400 && status < 500 {
				code = status
			}
			http.Error(w, "upstream /info: "+err.Error(), code)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(infoResponse{
			Edge: edgeMetadata{
				Version:         edgeVersion,
				ProtocolVersion: edgeProtocolVersion,
			},
			Operator: got,
		})
	}
}
