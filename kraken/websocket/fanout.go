package websocket

import (
	"sync"
	"sync/atomic"
)

/*
channelHandler pairs a stable subscription id with its callback so Unsubscribe
can drop one listener without comparing function values.
*/
type channelHandler struct {
	id uint64
	fn func([]byte)
}

/*
fanout is the shared channel callback bus for Live and Paper. Embed it so On,
Unsubscribe, and emit are the type's methods without passthrough wrappers.
*/
type fanout struct {
	mu       sync.Mutex
	handlers map[string][]channelHandler
	nextID   atomic.Uint64
}

/*
On registers a raw channel consumer and returns its unsubscribe id.
*/
func (fanout *fanout) On(channel string, action func([]byte)) uint64 {
	if fanout == nil || action == nil {
		return 0
	}

	id := fanout.nextID.Add(1)

	fanout.mu.Lock()
	defer fanout.mu.Unlock()

	fanout.handlers[channel] = append(
		fanout.handlers[channel],
		channelHandler{id: id, fn: action},
	)

	return id
}

/*
Unsubscribe removes one handler previously registered with On for channel.
*/
func (fanout *fanout) Unsubscribe(channel string, id uint64) {
	if fanout == nil || id == 0 {
		return
	}

	fanout.mu.Lock()
	defer fanout.mu.Unlock()

	fanout.handlers[channel] = dropHandler(fanout.handlers[channel], id)
}

/*
emit delivers raw to every handler on channel. Missing handlers return false
without logging — frames can arrive before a consumer registers.
*/
func (fanout *fanout) emit(channel string, raw []byte) bool {
	if fanout == nil {
		return false
	}

	fanout.mu.Lock()
	handlers := append([]channelHandler(nil), fanout.handlers[channel]...)
	fanout.mu.Unlock()

	if len(handlers) == 0 {
		return false
	}

	for _, handler := range handlers {
		handler.fn(raw)
	}

	return true
}

/*
dropHandler removes the channelHandler with the given id.
*/
func dropHandler(handlers []channelHandler, id uint64) []channelHandler {
	if id == 0 || len(handlers) == 0 {
		return handlers
	}

	next := make([]channelHandler, 0, len(handlers))

	for _, handler := range handlers {
		if handler.id == id {
			continue
		}

		next = append(next, handler)
	}

	return next
}
