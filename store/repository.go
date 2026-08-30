package store

import (
	"time"
)

/*
Repository is the common persistence boundary every store-backed component
writes through. It deliberately exposes the narrowest uniform write surface —
one entry per raw websocket frame, tagged by its origin kind and endpoint — so
the storage engine behind it (SQLite today, an S3-compatible object store later)
can be swapped without any pipeline wiring changing. Implementations own their
own durability, batching, and backpressure policy; the writer only reports.
*/
type Repository interface {
	// WriteFrame persists one raw transport frame. endpoint names the source
	// stream (the websocket URL); kind names the frame's channel/method/feed
	// (e.g. "ticker", "trade", "book", "level3"); payload is the exact bytes
	// off the wire, unmodified; at is the arrival instant.
	WriteFrame(endpoint, kind string, payload []byte, at time.Time) error

	// Close releases the store's underlying resources.
	Close() error
}
