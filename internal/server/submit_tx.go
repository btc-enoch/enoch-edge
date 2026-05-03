package server

import (
	"io"
	"net/http"

	"github.com/enoch/enoch-edge/internal/upstream"
)

// maxSubmitTxBody caps the request body at 1MB. Real Enoch txs are
// well under 100KB; the cap is purely a sanity guard against a wallet
// (or a probe) sending megabytes of garbage that we'd buffer in RAM
// before discovering the operator rejects it.
const maxSubmitTxBody = 1 << 20

// handleSubmitTx is a near-passthrough proxy for POST /submit_tx.
// Edge does not parse the body — the operator owns the wire format
// (hex-encoded txhash/scriptSig/scriptPubKey, etc.) and we do not
// want to keep edge's schema in lockstep with protocol changes.
//
// Status and body are forwarded verbatim so wallets see the
// operator's own error messages (e.g. "insufficient funds" → 422,
// "script verification failed" → 400).
//
// Single-op mode: takes a *Client directly. Federation mode
// (#102): handleSubmitTxFederation wraps a *FederationPool and
// transparently retries against the leader on 503 redirects.
func handleSubmitTx(op *upstream.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxSubmitTxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		status, respBody, err := op.PostJSON(req.Context(), "/submit_tx", body)
		if err != nil {
			http.Error(w, "upstream /submit_tx: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
	}
}

// handleSubmitTxFederation routes /submit_tx through a FederationPool
// so a wallet hitting any operator gets transparently redirected to
// the current leader. The pool absorbs the 503-redirect dance from
// spec/federation_lifecycle.md §"What if /submit_tx lands at a
// non-leader operator?".
func handleSubmitTxFederation(pool *upstream.FederationPool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxSubmitTxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		status, respBody, err := pool.SubmitTx(req.Context(), body)
		if err != nil {
			http.Error(w, "federation /submit_tx: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
	}
}
