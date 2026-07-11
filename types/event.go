package types

import "time"

/*
Event is one decoded market row tagged with the stream that produced it
and that stream's own arrival order, so events drained from every feed
can be merged into one deterministic sequence, ordered by event time,
stream, and sequence, before any signal measures them.
*/
type Event struct {
	Stream   string
	Sequence uint64
	At       time.Time
	Symbol   string
	Price    float64
	Row      any
}

/*
EventChunk is one symbol's events, already ordered by event time,
stream, and sequence, as observed by Watermark: the newest event time
seen across every stream in the drain cycle that produced this chunk.
Every signal measuring an event in this chunk sees the same frozen
CrossSection snapshot taken at that watermark.
*/
type EventChunk struct {
	Symbol    string
	Watermark time.Time
	Events    []Event
}

/*
Drainer decodes queued frames from one feed's stream into ordered
events, and measures one already-ordered event against a shared,
frozen CrossSection snapshot. Feeds implement this in addition to Feed
so a chunker can merge every stream's events before any signal runs,
instead of each feed measuring its own batch in isolation.
*/
type Drainer interface {
	Drain() ([]Event, error)
	MeasureEvent(Event, *CrossSection) ([]*Measurement, error)
}
