package transport_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestConn(t *testing.T) {
	Convey("Conn wraps a member pipeline as an addressable connectable endpoint", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		member := store.NewKeyed[float64]()
		seed := store.Slot[float64]{Value: 42.0}

		for range member.Next(sequence.NewOne(unsafe.Pointer(&seed)).Next(nil)) {
		}

		conn := transport.NewConn[*geometry.Coordinate](member)

		So(conn, ShouldNotBeNil)
		So(conn.Error(), ShouldBeNil)

		interests := [][]string{{"ticker", "data", "price"}}
		registration := nomagique.NewNumber(
			sequence.NewValues(interests),
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				conn, core.Identify,
			),
			grid,
		)

		publish := sequence.Read[core.Connectable[*geometry.Coordinate]](registration.Next(nil))
		So(publish, ShouldNotBeNil)
		So(conn.Identity(), ShouldNotBeNil)
		So(conn.Identity().X, ShouldEqual, 0)
		So(conn.Identity().Y, ShouldEqual, 0)

		readAddress := transport.NewAddress[*geometry.Coordinate]()
		readAddress.Identify(conn.Identity())
		reading := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				readAddress, core.Read,
			).Next(nil),
		))
		So(reading.Value, ShouldNotBeNil)
		So(*reading.Value, ShouldEqual, 42.0)
		So(reading.Origin.Identity(), ShouldEqual, conn.Identity())

		value := store.Slot[float64]{Value: 7.0}
		var received store.Slot[float64]
		for out := range conn.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
			received = *(*store.Slot[float64])(out)
		}

		So(received.Value, ShouldEqual, 7.0)

		reread := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				readAddress, core.Read,
			).Next(nil),
		))
		So(*reread.Value, ShouldEqual, 7.0)
	})
}
