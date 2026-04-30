package feeoracle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const mempoolBody = `{"fastestFee":25,"halfHourFee":12,"hourFee":5,"economyFee":2,"minimumFee":1}`

// TestGet_DecodesMempoolShape covers the happy path: upstream returns
// mempool.space's camelCase fields and we translate to our snake_case
// Rates struct.
func TestGet_DecodesMempoolShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mempoolBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 30*time.Second)
	snap, err := c.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Rates.Fastest != 25 || snap.Rates.HalfHour != 12 || snap.Rates.Hour != 5 || snap.Rates.Economy != 2 || snap.Rates.Minimum != 1 {
		t.Errorf("rates = %+v, want {25,12,5,2,1}", snap.Rates)
	}
	if snap.AsOf.IsZero() {
		t.Error("as_of unset")
	}
}

// TestGet_CachesWithinTTL ensures concurrent wallets behind edge
// don't each hit upstream — the second Get within the TTL should
// reuse the cached snapshot.
func TestGet_CachesWithinTTL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(mempoolBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 1*time.Hour) // long TTL, second Get must be cached
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (second Get must be cached)", got)
	}
}

// TestGet_StaleOnError keeps wallets working when mempool.space
// blips: if there's a previously-good snapshot, return it rather
// than failing. Wallet still gets a usable estimate; "as_of" lets
// the UI flag staleness if it cares.
func TestGet_StaleOnError(t *testing.T) {
	var phase atomic.Int32 // 0 = serve OK, 1 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if phase.Load() == 0 {
			_, _ = w.Write([]byte(mempoolBody))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, 1*time.Nanosecond) // force expiry between calls
	first, err := c.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	phase.Store(1)
	time.Sleep(2 * time.Millisecond) // ensure cache is "expired"
	second, err := c.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get returned err despite cached fallback: %v", err)
	}
	if second != first {
		t.Errorf("expected stale snapshot reuse on upstream error")
	}
}
