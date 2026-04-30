package eventbus

import "sync"

// subscriberBufSize buffers a small burst per wallet connection.
// Slow consumers miss events (drop) rather than block the bus.
const subscriberBufSize = 64

type subscription struct {
	filter map[string]struct{} // enoch1 strings; nil/empty = no filter
	ch     chan ProxyEvent
}

// Bus fans out ProxyEvents from the upstream client to local
// (wallet-facing) subscribers. Optional per-subscriber filter on
// enoch1 addresses — events whose address list intersects the
// filter pass through; events with no address list (e.g. state-root
// signed) always pass through, since wallets need them for
// verification regardless of which address they're watching.
type Bus struct {
	mu     sync.RWMutex
	nextID uint64
	subs   map[uint64]*subscription
}

func NewBus() *Bus {
	return &Bus{subs: map[uint64]*subscription{}}
}

// Subscribe with an optional set of enoch1 strings. The cleanup func
// MUST be called when the subscriber goes away (defer in the handler).
func (b *Bus) Subscribe(filter []string) (<-chan ProxyEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var fset map[string]struct{}
	if len(filter) > 0 {
		fset = make(map[string]struct{}, len(filter))
		for _, a := range filter {
			fset[a] = struct{}{}
		}
	}

	id := b.nextID
	b.nextID++
	sub := &subscription{filter: fset, ch: make(chan ProxyEvent, subscriberBufSize)}
	b.subs[id] = sub
	return sub.ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if s, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(s.ch)
		}
	}
}

// Publish delivers an event to subscribers whose filter matches.
// Non-blocking: full subscriber buffers drop the event.
func (b *Bus) Publish(ev ProxyEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		if !matches(sub.filter, ev.Addresses) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			// slow subscriber — drop
		}
	}
}

// matches: empty filter accepts all; events with no addresses
// (system-wide events like state-root signed) always pass.
func matches(filter map[string]struct{}, addrs []string) bool {
	if len(filter) == 0 {
		return true
	}
	if len(addrs) == 0 {
		return true
	}
	for _, a := range addrs {
		if _, ok := filter[a]; ok {
			return true
		}
	}
	return false
}
