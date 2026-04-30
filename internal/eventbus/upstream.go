package eventbus

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// UpstreamClient runs a long-lived SSE connection to the operator's
// /events stream and forwards each event into the local Bus.
//
// Lifecycle: one connection per edge process, started at boot, kept
// open across the edge's lifetime. On disconnect, reconnects with
// exponential backoff (1s → 30s cap) until the context is canceled.
type UpstreamClient struct {
	operatorURL string
	bus         *Bus
	httpC       *http.Client
	logger      *log.Logger
}

func NewUpstream(operatorURL string, bus *Bus, logger *log.Logger) *UpstreamClient {
	return &UpstreamClient{
		operatorURL: strings.TrimRight(operatorURL, "/"),
		bus:         bus,
		// No client timeout — SSE is long-lived. Disconnect detection
		// happens at the read level (EOF / error).
		httpC:  &http.Client{},
		logger: logger,
	}
}

// Run blocks until ctx is canceled, reconnecting on each disconnect.
func (u *UpstreamClient) Run(ctx context.Context) {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for {
		err := u.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			u.logger.Printf("eventbus upstream: %v (retry in %s)", err, backoff)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		// On a successful connect (we got past runOnce returning nil
		// or with EOF), reset backoff. Tracked via err==nil OR a
		// connection that lasted longer than the backoff itself.
		if err == nil {
			backoff = time.Second
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (u *UpstreamClient) runOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.operatorURL+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := u.httpC.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("operator /events: status %d", resp.StatusCode)
	}
	u.logger.Printf("eventbus upstream: connected to %s/events", u.operatorURL)
	return u.parseStream(resp.Body)
}

// parseStream reads SSE-format lines and emits one ProxyEvent per
// `event:`/`data:` pair (separated by a blank line per the spec).
// Comments (lines starting with `:`) are heartbeats — ignored.
func (u *UpstreamClient) parseStream(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1MB per line

	var eventType string
	var dataBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventType != "" && dataBuf.Len() > 0 {
				u.dispatch(eventType, []byte(dataBuf.String()))
			}
			eventType = ""
			dataBuf.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment / heartbeat
		}
		if rest, ok := strings.CutPrefix(line, "event: "); ok {
			eventType = rest
		} else if rest, ok := strings.CutPrefix(line, "data: "); ok {
			dataBuf.WriteString(rest)
		}
	}
	return scanner.Err()
}

func (u *UpstreamClient) dispatch(eventType string, data []byte) {
	u.bus.Publish(ProxyEvent{
		Type:      eventType,
		Addresses: extractAddresses(eventType, data),
		Raw:       data,
	})
}

// extractAddresses pulls the per-event address list out of `data`
// for filterable event types (currently tx_applied). Other event
// types deliver to all subscribers.
func extractAddresses(eventType string, data []byte) []string {
	if eventType != "tx_applied" {
		return nil
	}
	var d struct {
		Addresses []string `json:"addresses"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil
	}
	return d.Addresses
}
