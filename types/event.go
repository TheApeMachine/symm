package types

import (
	"sort"
	"time"
)

/*
Event is one decoded market row tagged with the stream that produced it
and that stream's own arrival order, so events drained from every feed
can be merged into one deterministic sequence, ordered by event time,
stream, and sequence, before any signal measures them.
*/
type Event struct {
	Stream   string
	Priority int
	Sequence uint64
	At       time.Time
	Symbol   string
	Price    float64
	Row      any
}

/*
OrderEvents sorts decoded rows by event time, explicit stream priority, stream
sequence, and symbol. Callers choose priority from signal semantics; sequence
preserves the authoritative order within each entity journal.
*/
func OrderEvents(events []Event) {
	sort.SliceStable(events, func(left int, right int) bool {
		leftEvent := events[left]
		rightEvent := events[right]

		if !leftEvent.At.Equal(rightEvent.At) {
			return leftEvent.At.Before(rightEvent.At)
		}

		if leftEvent.Priority != rightEvent.Priority {
			return leftEvent.Priority < rightEvent.Priority
		}

		if leftEvent.Sequence != rightEvent.Sequence {
			return leftEvent.Sequence < rightEvent.Sequence
		}

		return leftEvent.Symbol < rightEvent.Symbol
	})
}

/*
EventChunk is one symbol's events, already ordered by event time,
stream, and sequence, as observed by Watermark: the newest event time
seen across every stream in the drain cycle that produced this chunk.
Every signal measuring an event in this chunk contributes to the same
CrossSection carried by the current Thesis tick.
*/
type EventChunk struct {
	Symbol    string
	Watermark time.Time
	Events    []Event
}

/*
Drainer decodes queued frames from one feed's stream into ordered
events, and measures one already-ordered event against the shared current-tick
CrossSection. Feeds implement this in addition to Feed
so a chunker can merge every stream's events before any signal runs,
instead of each feed measuring its own batch in isolation.
*/
type Drainer interface {
	Drain() ([]Event, error)
	MeasureEvent(Event, *CrossSection) ([]*Measurement, error)
}
