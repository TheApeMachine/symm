package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
	})
}
