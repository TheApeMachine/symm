package types_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestRouteFilters(t *testing.T) {
	Convey("Given route filtering logic", t, func() {
		arena := data.NewArenaOwner("test", 1024)

		Convey("Fluid route rejects raw manifold measurements to prevent websocket buffer flooding", func() {
			types.SetRoute("fluid")
			measurement := arena.NewMeasurement(1, "BTC/USD", "manifold", 1, 1, nil)
			So(types.Filters(measurement), ShouldBeFalse)
		})

		Convey("Dashboard route filters by focus and analytical sources", func() {
			types.SetRoute("dashboard")
			types.SetFocus("BTC/USD")

			hawkes := arena.NewMeasurement(1, "BTC/USD", "hawkes", 1, 1, nil)
			So(types.Filters(hawkes), ShouldBeTrue)

			otherSymbol := arena.NewMeasurement(1, "ETH/USD", "hawkes", 1, 1, nil)
			So(types.Filters(otherSymbol), ShouldBeFalse)

			manifold := arena.NewMeasurement(1, "BTC/USD", "manifold", 1, 1, nil)
			So(types.Filters(manifold), ShouldBeFalse)
		})

		Convey("Learning route only admits training strategy outputs", func() {
			types.SetRoute("learning")

			training := arena.NewMeasurement(1, "BTC/USD", "training", 1, 1, nil)
			So(types.Filters(training), ShouldBeTrue)

			hawkes := arena.NewMeasurement(1, "BTC/USD", "hawkes", 1, 1, nil)
			So(types.Filters(hawkes), ShouldBeFalse)

			manifold := arena.NewMeasurement(1, "BTC/USD", "manifold", 1, 1, nil)
			So(types.Filters(manifold), ShouldBeFalse)
		})
	})
}
