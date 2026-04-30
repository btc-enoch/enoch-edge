package server

import (
	"encoding/json"
	"net/http"
)

// handleHealth is a local liveness check. Deliberately does NOT call
// upstream — k8s / load balancers should be able to mark edge healthy
// even if the operator is briefly unreachable, so traffic still
// drains gracefully via cached responses.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
