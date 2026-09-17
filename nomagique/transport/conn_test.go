package transport_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestConn(t *testing.T) {
	Convey("Conn registers with a grid once before streaming arrivals", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		conn := transport.NewConn[*geometry.Coordinate, [][]string](
			grid,
			[][]string{{"ticker", "data", "price"}},
		)

		So(conn, ShouldNotBeNil)
		So(conn.Error(), ShouldBeNil)

		value := 42.0
		var received float64
		for out := range conn.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
			received = *(*float64)(out)
		}

		So(received, ShouldEqual, 42.0)
		So(conn.Identity(), ShouldNotBeNil)
		So(conn.Identity().X, ShouldEqual, 0)
		So(conn.Identity().Y, ShouldEqual, 0)
	})
}

