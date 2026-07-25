package types

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChunkEventsBySymbol(t *testing.T) {
	Convey("Given globally ordered multi-symbol events", t, func() {
		events := []Event{
			{Stream: "trade", Symbol: "A/USD", Sequence: 1},
			{Stream: "book", Symbol: "B/USD", Sequence: 1},
			{Stream: "trade", Symbol: "A/USD", Sequence: 2},
			{Stream: "book", Symbol: "C/USD", Sequence: 1},
			{Stream: "book", Symbol: "B/USD", Sequence: 2},
		}

		chunks := ChunkEventsBySymbol(events)

		Convey("It preserves first-seen symbol order and per-symbol event order", func() {
			So(chunks, ShouldHaveLength, 3)
			So(chunks[0].Symbol, ShouldEqual, "A/USD")
			So(chunks[0].Events, ShouldHaveLength, 2)
			So(chunks[0].Events[0].Sequence, ShouldEqual, 1)
			So(chunks[0].Events[1].Sequence, ShouldEqual, 2)
			So(chunks[1].Symbol, ShouldEqual, "B/USD")
			So(chunks[1].Events, ShouldHaveLength, 2)
			So(chunks[2].Symbol, ShouldEqual, "C/USD")
			So(chunks[2].Events, ShouldHaveLength, 1)
		})
	})
}

func TestMeasureEventsParallel(t *testing.T) {
	Convey("Given two symbols whose measurement overlaps in time", t, func() {
		var active int32
		var peak int32
		var record sync.Mutex

		measure := func(event Event) ([]*Measurement, error) {
			current := atomic.AddInt32(&active, 1)

			record.Lock()

			if current > peak {
				peak = current
			}

			record.Unlock()

			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)

			return []*Measurement{{Symbol: event.Symbol}}, nil
		}

		events := []Event{
			{Stream: "book", Symbol: "A/USD", Sequence: 1},
			{Stream: "book", Symbol: "B/USD", Sequence: 1},
		}

		measurements, err := MeasureEventsParallel(events, measure)

		Convey("It completes both symbols without requiring global serial order", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 2)
			So(peak, ShouldBeGreaterThanOrEqualTo, 2)
		})
	})

	Convey("Given one symbol with ordered events", t, func() {
		seen := make([]uint64, 0, 2)

		measure := func(event Event) ([]*Measurement, error) {
			seen = append(seen, event.Sequence)

			return nil, nil
		}

		events := []Event{
			{Stream: "book", Symbol: "A/USD", Sequence: 1},
			{Stream: "book", Symbol: "A/USD", Sequence: 2},
		}

		_, err := MeasureEventsParallel(events, measure)

		Convey("It keeps per-symbol event order", func() {
			So(err, ShouldBeNil)
			So(seen, ShouldResemble, []uint64{1, 2})
		})
	})
}

func TestRunSymbolGroupsParallel(t *testing.T) {
	Convey("Given two symbol row groups", t, func() {
		var gate sync.WaitGroup
		gate.Add(2)
		var peak int32
		var record sync.Mutex

		groups := []SymbolRows[int]{
			{Symbol: "A/USD", Rows: []int{1, 2}},
			{Symbol: "B/USD", Rows: []int{3}},
		}

		err := RunSymbolGroupsParallel(groups, func(index int, rows []int) error {
			gate.Done()
			gate.Wait()

			record.Lock()
			peak++
			record.Unlock()

			return nil
		})

		Convey("It runs distinct symbols concurrently", func() {
			So(err, ShouldBeNil)
			So(peak, ShouldEqual, 2)
		})
	})
}

func BenchmarkMeasureEventsParallel(b *testing.B) {
	events := make([]Event, 256)

	for index := range events {
		events[index] = Event{
			Stream:   "book",
			Symbol:   []string{"A/USD", "B/USD", "C/USD", "D/USD"}[index%4],
			Sequence: uint64(index/4 + 1),
		}
	}

	measure := func(event Event) ([]*Measurement, error) {
		return []*Measurement{{
			Symbol: event.Symbol,
			Metrics: map[string]MetricSample{
				MetricKey(MetricStrength, SideNone): {Raw: 1},
			},
		}}, nil
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = MeasureEventsParallel(events, measure)
	}
}
