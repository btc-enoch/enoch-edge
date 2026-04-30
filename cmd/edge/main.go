// Command edge runs the wallet-facing edge proxy in front of the
// Enoch operator(s). All actual work lives under internal/ — this
// file is just argument parsing, signal handling, and wiring.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/enoch/enoch-edge/internal/config"
	"github.com/enoch/enoch-edge/internal/eventbus"
	"github.com/enoch/enoch-edge/internal/feeoracle"
	"github.com/enoch/enoch-edge/internal/server"
	"github.com/enoch/enoch-edge/internal/upstream"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[edge] ", log.LstdFlags|log.Lmicroseconds)

	op := upstream.New(cfg.OperatorURL, 5*time.Second)
	oracle := feeoracle.New(cfg.FeeOracleURL, cfg.FeeOracleTTL)
	bus := eventbus.NewBus()
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           server.New(logger, op, oracle, bus),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Long-lived upstream SSE connection — one per edge process,
	// fans out to wallet subscribers via the bus.
	upstreamCtx, upstreamCancel := context.WithCancel(context.Background())
	defer upstreamCancel()
	go eventbus.NewUpstream(cfg.OperatorURL, bus, logger).Run(upstreamCtx)

	logger.Printf("starting on %s, upstream operator=%s, fee oracle=%s", cfg.Listen, cfg.OperatorURL, cfg.FeeOracleURL)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown so SSE clients get a clean close on SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
