package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/enoch/enoch-edge/internal/address"
	"github.com/enoch/enoch-edge/internal/eventbus"
)

// edgeSSEHeartbeat keeps wallet-facing SSE connections alive through
// CDNs / load balancers that cull silent streams. Same tradeoff as
// the operator side: short enough to beat typical idle timeouts,
// cheap enough on the wire to scale to many wallets.
const edgeSSEHeartbeat = 15 * time.Second

// handleEvents serves /v1/events as a Server-Sent Events stream.
//
// Optional ?addr=... params (any address format) limit the stream
// to events touching those pkhs. Multi-format input is normalized
// here, same as for the read endpoints. Events without an address
// list (state-root signed) always pass through — wallets need them
// for light-client verification regardless of which address they
// happen to be watching.
func handleEvents(bus *eventbus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Normalize wallet's address filter to the enoch1 form the
		// upstream events use. Decoding before we set SSE headers
		// means a bad input gets a JSON 400, not a half-open stream.
		raw := req.URL.Query()["addr"]
		filter := make([]string, 0, len(raw))
		for _, a := range raw {
			pkh, err := address.DecodeToPKH(a)
			if err != nil {
				http.Error(w, "decode addr "+a+": "+err.Error(), http.StatusBadRequest)
				return
			}
			enoch, err := address.EncodeEnoch(pkh)
			if err != nil {
				http.Error(w, "encode enoch: "+err.Error(), http.StatusInternalServerError)
				return
			}
			filter = append(filter, enoch)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		ch, cleanup := bus.Subscribe(filter)
		defer cleanup()

		ticker := time.NewTicker(edgeSSEHeartbeat)
		defer ticker.Stop()

		for {
			select {
			case <-req.Context().Done():
				return
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Raw); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
