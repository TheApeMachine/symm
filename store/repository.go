package store

import (
	"time"
)

/*
Repository is the common persistence boundary every store-backed component
writes through. It deliberately exposes the narrowest uniform write surface —
one entry per recorded fact, tagged by kind — so the storage engine behind it
(SQLite today, an S3-compatible object store later) can be swapped without any
pipeline wiring changing. Implementations own their own durability, batching,
and backpressure policy; the writer only reports.
*/
type Repository interface {
	// WriteEvent persists one kind-tagged record. kind names the event domain
	// ("websocket_frame", "envelope", "strategy", ...); payload is the
	// already-encoded bytes the producer wants preserved verbatim. at is the
	// arrival/validity instant.
	WriteEvent(kind string, payload []byte, at time.Time) error

	// Close releases the store's underlying resources.
	Close() error
}
