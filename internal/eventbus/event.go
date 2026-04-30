// Package eventbus is edge's in-process pub/sub for protocol events
// streamed from the operator. One upstream SSE connection feeds the
// bus; many wallet SSE connections subscribe with optional address
// filters.
package eventbus

// ProxyEvent is what the upstream client publishes and the wallet
// SSE handler forwards. Raw is the operator's data verbatim — we
// don't re-encode on the way through, so wallets see exactly what
// the operator sent.
type ProxyEvent struct {
	Type      string
	Addresses []string // pre-extracted from Raw for filtering; empty = unfiltered event type
	Raw       []byte   // operator's `data:` payload, JSON
}
