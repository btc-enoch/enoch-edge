package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestGetJSON_Success uses httptest to fake an operator and verifies
// the happy path: 200 with a JSON body decodes into out.
func TestGetJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"network":"regtest","current_height":42}`))
	}))
	defer srv.Close()

	c := New(srv.URL, 2*time.Second)

	var out map[string]any
	status, err := c.GetJSON(context.Background(), "/info", &out)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if got := out["network"]; got != "regtest" {
		t.Errorf("network = %v, want regtest", got)
	}
}

// TestPostJSON_ForwardsBodyOnError confirms PostJSON's pass-through
// semantics: a 4xx upstream is NOT a Go error, the body is returned
// so the wallet can see the operator's actual rejection reason.
func TestPostJSON_ForwardsBodyOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		http.Error(w, "insufficient funds", http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := New(srv.URL, 2*time.Second)

	status, body, err := c.PostJSON(context.Background(), "/submit_tx", []byte(`{"hi":1}`))
	if err != nil {
		t.Fatalf("PostJSON: unexpected err %v (4xx should not be a Go error)", err)
	}
	if status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", status)
	}
	if want := "insufficient funds"; !strings.Contains(string(body), want) {
		t.Errorf("body = %q, want to contain %q", body, want)
	}
}

// TestPostJSON_Success forwards the operator's success body verbatim
// (including tx_hash, etc.) so the wallet sees the operator's exact
// JSON shape with no edge-side reshaping on the write path.
func TestPostJSON_Success(t *testing.T) {
	const upstreamBody = `{"status":"ok","tx_hash":"deadbeef","burns":0}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstreamBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 2*time.Second)
	status, body, err := c.PostJSON(context.Background(), "/submit_tx", []byte(`{}`))
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if string(body) != upstreamBody {
		t.Errorf("body = %q, want %q", body, upstreamBody)
	}
}

// TestGetJSON_4xx confirms the 4xx status is surfaced verbatim so
// the edge can pass the same status back to the wallet.
func TestGetJSON_4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, 2*time.Second)

	var out map[string]any
	status, err := c.GetJSON(context.Background(), "/missing", &out)
	if err == nil {
		t.Fatal("GetJSON: expected error, got nil")
	}
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}
