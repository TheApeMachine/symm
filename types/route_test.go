package types_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestRouteFilters(t *testing.T) {
	Convey("Given route filtering logic", t, func() {
		Convey("Fluid route rejects raw manifold measurements to prevent websocket buffer flooding", func() {
			types.SetRoute("fluid")
			measurement := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			So(types.Filters(measurement), ShouldBeFalse)
		})

		Convey("Dashboard route filters by focus and analytical sources", func() {
			types.SetRoute("dashboard")
			types.SetFocus("BTC/USD")

			hawkes := data.NewMeasurement(1, "BTC/USD", "hawkes", 1, 1)
			So(types.Filters(hawkes), ShouldBeTrue)

			otherSymbol := data.NewMeasurement(1, "ETH/USD", "hawkes", 1, 1)
			So(types.Filters(otherSymbol), ShouldBeFalse)

			manifold := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			So(types.Filters(manifold), ShouldBeFalse)
		})

		Convey("Learning route only admits training strategy outputs", func() {
			types.SetRoute("learning")

			training := data.NewMeasurement(1, "BTC/USD", "training", 1, 1)
			So(types.Filters(training), ShouldBeTrue)

			hawkes := data.NewMeasurement(1, "BTC/USD", "hawkes", 1, 1)
			So(types.Filters(hawkes), ShouldBeFalse)

			manifold := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			So(types.Filters(manifold), ShouldBeFalse)
		})

		Convey("Diagnostics route admits focused measurements for system visibility", func() {
			types.SetRoute("diagnostics")
			types.SetFocus("BTC/USD")

			cvd := data.NewMeasurement(1, "BTC/USD", "cvd", 1, 1)
			So(types.Filters(cvd), ShouldBeTrue)

			manifold := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			So(types.Filters(manifold), ShouldBeTrue)

			other := data.NewMeasurement(1, "ETH/USD", "cvd", 1, 1)
			So(types.Filters(other), ShouldBeFalse)
		})
	})
}
