// Package server wires the wallet-facing HTTP routes. Versioning
// lives at the edge under /v1/, not at the operator, so response
// shapes can change without touching protocol code.
package server

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/enoch/enoch-edge/internal/eventbus"
	"github.com/enoch/enoch-edge/internal/feeoracle"
	"github.com/enoch/enoch-edge/internal/upstream"
)

// New returns the edge's top-level http.Handler.
func New(logger *log.Logger, op *upstream.Client, oracle *feeoracle.Client, bus *eventbus.Bus) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(loggerMiddleware(logger))
	r.Use(jsonContentType)

	r.Get("/v1/health", handleHealth)
	r.Get("/v1/info", handleInfo(op))
	r.Get("/v1/utxos/{addr}", handleUTXOs(op))
	r.Get("/v1/balance/{addr}", handleBalance(op))
	r.Get("/v1/address_history/{addr}", handleAddressHistory(op))
	r.Get("/v1/fee_oracle", handleFeeOracle(oracle))
	r.Get("/v1/events", handleEvents(bus))
	r.Post("/v1/submit_tx", handleSubmitTx(op))

	return r
}

// jsonContentType sets a default JSON content type for handlers that
// don't override it. SSE handlers will overwrite this header before
// writing their first event.
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, req)
	})
}

// loggerMiddleware is a thin access-log wrapper that routes through
// the injected log.Logger so deployments can swap the sink (stdout
// vs. journal vs. structured pipeline).
func loggerMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(ww, req)
			logger.Printf("%s %s -> %d (%s)", req.Method, req.URL.Path, ww.Status(), time.Since(start))
		})
	}
}
