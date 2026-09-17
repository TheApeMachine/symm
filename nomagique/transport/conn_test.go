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
		member := store.NewRetained(42.0)
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

		value := 7.0
		var received float64
		for out := range conn.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
			received = *(*float64)(out)
		}

		So(received, ShouldEqual, 7.0)

		reread := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				readAddress, core.Read,
			).Next(nil),
		))
		So(*reread.Value, ShouldEqual, 7.0)
	})
}
