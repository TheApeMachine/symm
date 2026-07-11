package trader

import (
	"sort"

	"github.com/theapemachine/symm/types"
)

/*
Chunker merges every stream's drained events into stable, per-symbol
EventChunks ordered by event time, stream, and sequence, then freezes
one immutable CrossSection snapshot for the whole drain cycle so every
signal measuring any event from this cycle, on any stream, sees the
same frozen cross-sectional view instead of five feeds independently
draining their own rings in a fixed, timestamp-blind order.
*/
type Chunker struct {
	streams      map[string]types.Drainer
	order        []string
	crossSection *types.CrossSection
}

/*
NewChunker wires a Chunker around streams, keyed by the stream name
carried on every Event that stream drains, in order so streams whose
events tie on event time still merge deterministically.
*/
func NewChunker(
	crossSection *types.CrossSection, streams map[string]types.Drainer, order []string,
) *Chunker {
	return &Chunker{streams: streams, order: order, crossSection: crossSection}
}

/*
Drain pulls every stream's queued events in registration order, then
stably sorts the merged result into per-symbol EventChunks ordered by
event time, stream, and sequence. The returned snapshot is frozen
immediately after draining, so it reflects every ticker row any stream
folded into the shared CrossSection during this cycle and nothing
observed afterward — the watermark for every chunk this call returns.
*/
func (chunker *Chunker) Drain() ([]*types.EventChunk, *types.CrossSection, error) {
	events := make([]types.Event, 0)

	for _, stream := range chunker.order {
		drained, err := chunker.streams[stream].Drain()

		if err != nil {
			return nil, nil, err
		}

		events = append(events, drained...)
	}

	snapshot := chunker.crossSection.Snapshot()

	if len(events) == 0 {
		return nil, snapshot, nil
	}

	sort.SliceStable(events, func(i, j int) bool {
		return lessEvent(events[i], events[j])
	})

	return groupBySymbol(events), snapshot, nil
}

/*
Measure dispatches every event in chunks to the stream that produced it,
using the one shared snapshot for every call so no signal in this cycle
observes a partially updated cross-section, unlike measuring each
stream's full batch in isolation before moving to the next.
*/
func (chunker *Chunker) Measure(
	chunks []*types.EventChunk, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, chunk := range chunks {
		for _, event := range chunk.Events {
			drainer, ok := chunker.streams[event.Stream]

			if !ok {
				continue
			}

			result, err := drainer.MeasureEvent(event, snapshot)

			if err != nil {
				return nil, err
			}

			measurements = append(measurements, result...)
		}
	}

	return measurements, nil
}

func lessEvent(left, right types.Event) bool {
	if !left.At.Equal(right.At) {
		return left.At.Before(right.At)
	}

	if left.Stream != right.Stream {
		return left.Stream < right.Stream
	}

	return left.Sequence < right.Sequence
}

func groupBySymbol(events []types.Event) []*types.EventChunk {
	chunks := map[string]*types.EventChunk{}
	symbols := make([]string, 0)
	watermark := events[len(events)-1].At

	for _, event := range events {
		chunk, exists := chunks[event.Symbol]

		if !exists {
			chunk = &types.EventChunk{Symbol: event.Symbol, Watermark: watermark}
			chunks[event.Symbol] = chunk
			symbols = append(symbols, event.Symbol)
		}

		chunk.Events = append(chunk.Events, event)
	}

	sort.Strings(symbols)
	ordered := make([]*types.EventChunk, 0, len(symbols))

	for _, symbol := range symbols {
		ordered = append(ordered, chunks[symbol])
	}

	return ordered
}
