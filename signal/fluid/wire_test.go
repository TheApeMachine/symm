package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWireRow(t *testing.T) {
	Convey("Given a dashboard row map", t, func() {
		row := map[string]any{"symbol": "BTC/EUR", "value": 0.5}

		Convey("It should pass through the wire shape", func() {
			So(WireRow(row), ShouldResemble, row)
		})
	})
}
