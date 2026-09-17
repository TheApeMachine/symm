package store_test

import (
	"fmt"
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

type take struct {
	*core.PrimitiveError
	out float64
}

func (take *take) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil || input.Value == nil {
				continue
			}

			value, isFloat := (*input.Value).(float64)

			if !isFloat {
				continue
			}

			take.out = value

			if !yield(unsafe.Pointer(&take.out)) {
				return
			}
		}
	}
}

func TestGridDynamicAssignment(t *testing.T) {
	Convey("Grid dynamically assigns coordinates and communication pipes to unaddressed cells", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()

		firstInterests := [][]string{{"ticker", "data", "price"}}
		firstMember := &tests.Member[*geometry.Coordinate]{Primitive: store.NewRetained(1.0)}
		firstQuery := store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			firstMember, core.Identify,
		)

		endpoint := sequence.Read[core.Connectable[*geometry.Coordinate]](
			grid.Next(firstQuery.Next(sequence.NewValue(firstInterests))),
		)
		So(endpoint, ShouldNotBeNil)

		So(firstMember.Identity(), ShouldNotBeNil)
		So(firstMember.Identity().X, ShouldEqual, 0)
		So(firstMember.Identity().Y, ShouldEqual, 0)
		So(firstMember.Conn, ShouldNotBeNil)

		secondInterests := [][]string{{"ticker", "data", "qty"}}
		secondMember := &tests.Member[*geometry.Coordinate]{Primitive: store.NewRetained(2.0)}
		secondQuery := store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			secondMember, core.Identify,
		)

		endpoint2 := sequence.Read[core.Connectable[*geometry.Coordinate]](
			grid.Next(secondQuery.Next(sequence.NewValue(secondInterests))),
		)
		So(endpoint2, ShouldNotBeNil)

		So(secondMember.Identity(), ShouldNotBeNil)
		So(secondMember.Identity().X, ShouldEqual, 1)
		So(secondMember.Identity().Y, ShouldEqual, 0)
		So(secondMember.Conn, ShouldNotBeNil)

		readAddr := transport.NewAddress[*geometry.Coordinate]()
		readAddr.Identify(firstMember.Identity())
		readQuery := store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](readAddr, core.Read)

		reading := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(readQuery.Next(nil)))
		So(reading.Value, ShouldNotBeNil)
		So(*reading.Value, ShouldEqual, 1.0)
		So(reading.Origin.Identity(), ShouldEqual, firstMember.Identity())
	})
}

func TestGridNext(t *testing.T) {
	Convey("Grid routes keyed market inputs only to matching interests", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		last := store.NewRetained[float64]()
		bid := store.NewRetained[float64]()
		lastConn := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(&take{PrimitiveError: core.NewPrimitiveError()}, last),
		)
		bidConn := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(&take{PrimitiveError: core.NewPrimitiveError()}, bid),
		)

		sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
			store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				lastConn, core.Identify,
			).Next(sequence.NewValue([][]string{{"ticker", "data", "last"}})),
		))
		sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
			store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				bidConn, core.Identify,
			).Next(sequence.NewValue([][]string{{"ticker", "data", "bid"}})),
		))

		origin := transport.NewAddress[string]()
		origin.Identify("ETH/USD")
		lastVal := any(150.0)
		bidVal := any(149.5)
		lastInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "last"}, &lastVal,
		)
		bidInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "bid"}, &bidVal,
		)

		query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			nil, core.Write,
		)
		for range grid.Next(query.Next(sequence.NewValue(*lastInput, *bidInput))) {
		}

		lastReading := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				lastConn, core.Read,
			).Next(nil),
		))
		So(lastReading.Value, ShouldNotBeNil)
		lastSeen := *lastReading.Value
		bidReading := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				bidConn, core.Read,
			).Next(nil),
		))
		So(bidReading.Value, ShouldNotBeNil)

		So(lastSeen, ShouldEqual, 150.0)
		So(*bidReading.Value, ShouldEqual, 149.5)

		again := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				lastConn, core.Read,
			).Next(nil),
		))
		So(*again.Value, ShouldEqual, 150.0)
	})
}

func TestGridConcurrentReadsAndWrites(t *testing.T) {
	Convey("Grid writes and reads metrics concurrently via errgroup", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		const cellCount = 10
		conns := make([]*transport.Conn[*geometry.Coordinate], cellCount)

		for index := 0; index < cellCount; index++ {
			retained := store.NewRetained[float64]()
			key := fmt.Sprintf("metric_%d", index)
			conn := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(&take{PrimitiveError: core.NewPrimitiveError()}, retained),
			)
			conns[index] = conn

			sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
				store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				).Next(sequence.NewValue([][]string{{"test", "data", key}})),
			))
		}

		origin := transport.NewAddress[string]()
		origin.Identify("TEST/USD")
		var inputs []core.Input[string, []string, any]

		for index := 0; index < cellCount; index++ {
			key := fmt.Sprintf("metric_%d", index)
			val := any(float64(index * 10))
			inputs = append(inputs, *core.NewInput[string, []string, any](
				origin, core.Write, []string{"test", "data", key}, &val,
			))
		}

		writeQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			nil, core.Write,
		)
		for range grid.Next(writeQuery.Next(sequence.NewValue(inputs...))) {
		}

		readQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			nil, core.Read,
		)
		received := make(map[float64]bool)

		for reading := range grid.Next(readQuery.Next(nil)) {
			input := (*core.Input[*geometry.Coordinate, string, float64])(reading)
			if input != nil && input.Value != nil {
				received[*input.Value] = true
			}
		}

		So(len(received), ShouldEqual, cellCount)

		for index := 0; index < cellCount; index++ {
			expected := float64(index * 10)
			So(received[expected], ShouldBeTrue)
		}
	})

	Convey("Multiple goroutines concurrently writing to grid do not race or collide", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		const cellCount = 5
		for index := 0; index < cellCount; index++ {
			retained := store.NewRetained[float64]()
			key := fmt.Sprintf("hammer_%d", index)
			conn := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(&take{PrimitiveError: core.NewPrimitiveError()}, retained),
			)

			sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
				store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				).Next(sequence.NewValue([][]string{{"hammer", "data", key}})),
			))
		}

		origin := transport.NewAddress[string]()
		origin.Identify("HAMMER/USD")

		const workerCount = 8
		const iterationsPerWorker = 25
		done := make(chan struct{}, workerCount)

		for worker := 0; worker < workerCount; worker++ {
			workerID := worker
			go func() {
				defer func() { done <- struct{}{} }()

				for iterIdx := 0; iterIdx < iterationsPerWorker; iterIdx++ {
					targetCell := iterIdx % cellCount
					key := fmt.Sprintf("hammer_%d", targetCell)
					val := any(float64(workerID*1000 + iterIdx))
					input := core.NewInput[string, []string, any](
						origin, core.Write, []string{"hammer", "data", key}, &val,
					)

					query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
						nil, core.Write,
					)

					for range grid.Next(query.Next(sequence.NewValue(*input))) {
					}
				}
			}()
		}

		for worker := 0; worker < workerCount; worker++ {
			<-done
		}

		readQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			nil, core.Read,
		)
		readCount := 0

		for reading := range grid.Next(readQuery.Next(nil)) {
			input := (*core.Input[*geometry.Coordinate, string, float64])(reading)
			if input != nil && input.Value != nil {
				readCount++
			}
		}

		So(readCount, ShouldEqual, cellCount)
	})
}

