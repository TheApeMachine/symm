package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

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
		readQuery := store.NewQuery[*geometry.Coordinate, core.Primitive](readAddr, core.Read)

		readMember := sequence.Read[core.Primitive](grid.Next(readQuery.Next(nil)))
		So(readMember, ShouldEqual, firstMember)
	})
}

