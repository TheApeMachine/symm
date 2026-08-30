package store

import (
	"time"
)

/*
Writer is the store's transport-facing recorder. It turns one raw websocket
frame into a Repository WriteFrame, and is the single CaptureSink handed to every
stream so each frame is persisted once, verbatim, before any pipeline mutation.
*/
type Writer struct {
	repository Repository
}

/*
NewWriter builds a Writer over an already-open repository.
*/
func NewWriter(repository Repository) *Writer {
	return &Writer{repository: repository}
}

/*
Capture satisfies kraken.websocket.CaptureSink: it persists one untouched
transport payload with its origin kind, endpoint, and arrival time. kind names
the frame's channel/method/feed (e.g. "ticker", "trade", "book", "level3"); the
endpoint identifies the stream (public/private/level3/futures) in its own column.
Both are stored separately, so the payload is the exact bytes off the wire —
no URL or tag is prepended, and no derived envelope is written.
*/
func (writer *Writer) Capture(kind, endpoint string, payload []byte, receivedAt time.Time) error {
	if writer == nil || writer.repository == nil {
		return nil
	}

	return writer.repository.WriteFrame(
		endpoint,
		kind,
		payload,
		receivedAt,
	)
}
