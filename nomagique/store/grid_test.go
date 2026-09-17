package store_test

import (
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
			nil, core.Execute,
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
