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
		member := transport.NewIO[any](nil, nil)
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
		cell := sequence.Read[core.Primitive](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Primitive](readAddress, core.Read).Next(nil),
		))
		So(cell, ShouldEqual, conn)

		value := 42.0
		var received float64
		for out := range conn.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
			received = *(*float64)(out)
		}

		So(received, ShouldEqual, 42.0)

		execAddress := transport.NewAddress[*geometry.Coordinate]()
		execAddress.Identify(conn.Identity())
		executed := sequence.Read[float64](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				execAddress, core.Execute,
			).Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)),
		))
		So(executed, ShouldEqual, 42.0)
	})
}
