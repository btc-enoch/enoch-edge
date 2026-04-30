// Package config loads runtime configuration from the environment.
// Kept tiny on purpose — env vars only, no flag parsing — so the
// 12-factor "config in env" contract is the single source of truth.
package config

import (
	"os"
	"time"
)

type Config struct {
	Listen        string
	OperatorURL   string
	FeeOracleURL  string
	FeeOracleTTL  time.Duration
}

// Load reads the edge's environment, applying sensible dev defaults.
// host.docker.internal is the macOS / Docker Desktop bridge to the
// host network; on Linux the compose `extra_hosts` entry maps it to
// the host gateway so this works there too.
func Load() Config {
	return Config{
		Listen:       getenv("EDGE_LISTEN", ":8081"),
		OperatorURL:  getenv("OPERATOR_URL", "http://host.docker.internal:8080"),
		FeeOracleURL: getenv("FEE_ORACLE_URL", "https://mempool.space/api/v1/fees/recommended"),
		FeeOracleTTL: 30 * time.Second,
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
