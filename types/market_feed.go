package types

import (
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/datura/structure"
)

/*
MarketFeed is one bounded, single-consumer market-data journal. ClockRing owns
retention and ingress ordering; MarketFeed owns the central consumer's captured
cut and cursor.
*/
type MarketFeed[T any] struct {
	clock    *structure.ClockRing[string, T]
	cursor   structure.ClockCursor
	cut      structure.ClockCut
	captured bool
	err      error
}

/*
MarketBatch is one non-destructive event read and the exact cursor transition
that becomes valid only after a signal finishes processing every merged stream.
*/
type MarketBatch[T any] struct {
	Rows    []T
	From    uint64
	Through uint64
}

/*
NewMarketFeed constructs a journal with independently configured global and
per-symbol capacities so a large market universe does not multiply the global
burst allowance across every track.
*/
func NewMarketFeed[T any](
	timelineCapacity int,
	trackCapacity int,
) *MarketFeed[T] {
	if timelineCapacity <= 0 || trackCapacity <= 0 {
		return &MarketFeed[T]{err: fmt.Errorf(
			"types: market feed capacities must be positive: timeline=%d track=%d",
			timelineCapacity,
			trackCapacity,
		)}
	}

	if trackCapacity&(trackCapacity-1) != 0 {
		return &MarketFeed[T]{err: fmt.Errorf(
			"types: market feed track capacity must be a power of two: %d",
			trackCapacity,
		)}
	}

	// SPSC rings are lock-free for the clock's one-producer (Observe) /
	// many-walker (Batch, Frame) access pattern. Drop-oldest keeps ingest
	// non-blocking; capacities must be a power of two.
	timeline := structure.NewSPSCRing[structure.ClockSlot[string, T]](timelineCapacity, true)
	newTrack := func() structure.Ring[structure.ClockSlot[string, T]] {
		return structure.NewSPSCRing[structure.ClockSlot[string, T]](trackCapacity, true)
	}

	if timeline == nil {
		return &MarketFeed[T]{err: fmt.Errorf(
			"types: market feed timeline capacity must be a power of two: %d",
			timelineCapacity,
		)}
	}

	clock, err := structure.NewClockRing(timeline, newTrack)

	if err != nil {
		return &MarketFeed[T]{err: err}
	}

	return &MarketFeed[T]{clock: clock}
}

/*
Observe appends one authoritative row using event time for model ordering and
the clock's global ingress sequence for measurement-cycle membership.
*/
func (feed *MarketFeed[T]) Observe(
	symbol string,
	at time.Time,
	row T,
) error {
	if feed == nil {
		return fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return feed.err
	}

	if symbol == "" {
		return fmt.Errorf("types: market feed symbol is required")
	}

	return feed.clock.Observe(symbol, at, row)
}

/*
Capture freezes this consumer at the clock's present ingress high-water mark so
the central feed can cut every stream before concurrent measurement begins.
*/
func (feed *MarketFeed[T]) Capture(at time.Time) error {
	if feed == nil {
		return fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return feed.err
	}

	cut, err := feed.clock.Cut(at)

	if err != nil {
		return err
	}

	feed.cut = cut
	feed.captured = true

	return nil
}

/*
Drain returns every unseen row through the captured cut in ingress order and
advances the logical cursor only after the complete retained range is read.
*/
func (feed *MarketFeed[T]) Drain(at time.Time) ([]T, error) {
	batch, err := feed.Batch(at)

	if err != nil {
		return nil, err
	}

	if err := feed.Commit(batch); err != nil {
		return nil, err
	}

	return batch.Rows, nil
}

/*
Batch reads every unseen row through the captured cut without advancing the
cursor. Multi-stream signals commit all returned batches only after their
chronological merge has been processed.
*/
func (feed *MarketFeed[T]) Batch(at time.Time) (MarketBatch[T], error) {
	if feed == nil {
		return MarketBatch[T]{}, fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return MarketBatch[T]{}, feed.err
	}

	cut, err := feed.cutThrough(at)

	if err != nil {
		return MarketBatch[T]{}, err
	}

	slots, next, err := feed.clock.EventsAfter(feed.cursor, cut)

	if err != nil {
		return MarketBatch[T]{}, err
	}

	return MarketBatch[T]{
		Rows:    feed.payloads(slots),
		From:    feed.cursor.After,
		Through: next.After,
	}, nil
}

/*
Commit advances this consumer after its batch has been processed. The expected
starting cursor prevents stale or duplicate batches from being committed.
*/
func (feed *MarketFeed[T]) Commit(batch MarketBatch[T]) error {
	if feed == nil {
		return fmt.Errorf("types: market feed is required")
	}

	if feed.cursor.After != batch.From {
		return fmt.Errorf(
			"types: market batch starts at %d, current cursor is %d",
			batch.From,
			feed.cursor.After,
		)
	}

	feed.cursor.After = batch.Through

	return nil
}

/*
Progress reports whether the captured cut contains ingress newer than this
consumer's cursor. Market.Cut uses it so a quiet cycle can stay empty while a
book-only cycle can attach retained tickers for the active event symbols.
*/
func (feed *MarketFeed[T]) Progress() bool {
	if feed == nil || !feed.captured {
		return false
	}

	return feed.cut.Through != feed.cursor.After
}

/*
Frame returns one newest state per observed symbol through the captured cut.
Unlike Batch, it resurfaces retained latest rows even when this stream had no
new ingress. Market.Cut may then narrow that surface to active event symbols
so quote signals measure what changed rather than the whole universe.
The cursor advances to the cut high-water so event readers do not replay the
same ingress.
*/
func (feed *MarketFeed[T]) Frame(at time.Time) ([]T, error) {
	if feed == nil {
		return nil, fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return nil, feed.err
	}

	cut, err := feed.cutThrough(at)

	if err != nil {
		return nil, err
	}

	frame, err := feed.clock.FrameThrough(cut)

	if err != nil {
		return nil, err
	}

	slots := make([]structure.ClockSlot[string, T], 0, len(frame.Tracks))

	for _, slot := range frame.Tracks {
		slots = append(slots, slot)
	}

	sort.Slice(slots, func(left int, right int) bool {
		return slots[left].IngestSequence < slots[right].IngestSequence
	})
	feed.cursor.After = cut.Through

	return feed.payloads(slots), nil
}

/*
FrameSymbols returns the newest retained row for each requested symbol through
the captured cut, materializing only those tracks instead of the whole quote
universe Frame walks. Market.Cut uses it when only trades or books advanced so a
book-only cycle does not resurface every symbol's ticker. The cursor advances to
the cut high-water so event readers do not replay the same ingress.
*/
func (feed *MarketFeed[T]) FrameSymbols(
	at time.Time,
	symbols []string,
) ([]T, error) {
	if feed == nil {
		return nil, fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return nil, feed.err
	}

	cut, err := feed.cutThrough(at)

	if err != nil {
		return nil, err
	}

	slots := make([]structure.ClockSlot[string, T], 0, len(symbols))

	for _, symbol := range symbols {
		slot, found := feed.newestThrough(symbol, cut)

		if !found {
			continue
		}

		slots = append(slots, slot)
	}

	sort.Slice(slots, func(left int, right int) bool {
		return slots[left].IngestSequence < slots[right].IngestSequence
	})
	feed.cursor.After = cut.Through

	return feed.payloads(slots), nil
}

/*
newestThrough selects one symbol's newest retained observation at or before the
cut by walking only that symbol's track, so a narrowed cut never materializes the
full quote surface FrameThrough would build.
*/
func (feed *MarketFeed[T]) newestThrough(
	symbol string,
	cut structure.ClockCut,
) (structure.ClockSlot[string, T], bool) {
	track, ok := feed.clock.Track(symbol)

	if !ok {
		return structure.ClockSlot[string, T]{}, false
	}

	selected := structure.ClockSlot[string, T]{}
	found := false

	track.Select(0).Do(func(slot structure.ClockSlot[string, T]) {
		if slot.IngestSequence > cut.Through || slot.Wall.After(cut.At) {
			return
		}

		if found && slot.Wall.Before(selected.Wall) {
			return
		}

		if found && slot.Wall.Equal(selected.Wall) &&
			slot.IngestSequence < selected.IngestSequence {
			return
		}

		selected = slot
		found = true
	})

	return selected, found
}

/*
Pending returns unseen retained rows without moving the consumer cursor. Tests
and diagnostics can inspect logical queue state without clearing the journal.
*/
func (feed *MarketFeed[T]) Pending(at time.Time) ([]T, error) {
	if feed == nil {
		return nil, fmt.Errorf("types: market feed is required")
	}

	if feed.err != nil {
		return nil, feed.err
	}

	cut, err := feed.clock.Cut(at)

	if err != nil {
		return nil, err
	}

	slots, _, err := feed.clock.EventsAfter(feed.cursor, cut)

	if err != nil {
		return nil, err
	}

	return feed.payloads(slots), nil
}

/*
cutThrough consumes a planner-captured cut or captures the current high-water
for direct signal invocation in tests and replay tools.
*/
func (feed *MarketFeed[T]) cutThrough(at time.Time) (structure.ClockCut, error) {
	if feed.captured {
		feed.captured = false
		return feed.cut, nil
	}

	return feed.clock.Cut(at)
}

/*
payloads projects retained clock slots back into the decoded row values used by
existing signal calculations without changing their mathematical contracts.
*/
func (feed *MarketFeed[T]) payloads(
	slots []structure.ClockSlot[string, T],
) []T {
	rows := make([]T, 0, len(slots))

	for _, slot := range slots {
		rows = append(rows, slot.Payload)
	}

	return rows
}
