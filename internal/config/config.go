// Package config loads runtime configuration from the environment.
// Kept tiny on purpose — env vars only, no flag parsing — so the
// 12-factor "config in env" contract is the single source of truth.
package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Listen        string
	OperatorURL   string
	FeeOracleURL  string
	FeeOracleTTL  time.Duration

	// FederationURLs, when non-empty, switches edge into federation
	// mode. Indexed by operator_id (0..N-1). /submit_tx uses these
	// for leader-aware routing: try the last-known leader, on 503
	// read `leader_id` from the response body and retry against the
	// matching URL. Read endpoints continue to hit FederationURLs[0]
	// (state propagates across operators via quorum so any operator
	// is eventually consistent).
	//
	// Set via EDGE_FEDERATION_URLS as a comma-separated list, e.g.:
	//   EDGE_FEDERATION_URLS=http://host.docker.internal:18080,
	//                       http://host.docker.internal:18081,
	//                       http://host.docker.internal:18082
	//
	// When unset, edge falls back to OperatorURL — useful for dev
	// against a single specific federation operator (e.g. when
	// debugging one node) but not the canonical bring-up.
	FederationURLs []string
}

// Load reads the edge's environment, applying sensible dev defaults.
// host.docker.internal is the macOS / Docker Desktop bridge to the
// host network; on Linux the compose `extra_hosts` entry maps it to
// the host gateway so this works there too.
//
// Default OperatorURL points at federation operator 0's published
// port (18080); see docker-compose.federation.yml in the main repo.
func Load() Config {
	cfg := Config{
		Listen:       getenv("EDGE_LISTEN", ":8081"),
		OperatorURL:  getenv("OPERATOR_URL", "http://host.docker.internal:18080"),
		FeeOracleURL: getenv("FEE_ORACLE_URL", "https://mempool.space/api/v1/fees/recommended"),
		FeeOracleTTL: 30 * time.Second,
	}
	if raw := os.Getenv("EDGE_FEDERATION_URLS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				cfg.FederationURLs = append(cfg.FederationURLs, u)
			}
		}
	}
	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
