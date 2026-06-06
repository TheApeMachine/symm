package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/ring"
)

func TestExhaustMeasurement(t *testing.T) {
	Convey("Given warmed exit history", t, func() {
		history := symbolHistory{
			bidDepths:  ring.NewFloatRing(exitHistoryCap),
			spreads:    ring.NewFloatRing(exitHistoryCap),
			pressures:  ring.NewFloatRing(exitHistoryCap),
			imbalances: ring.NewFloatRing(exitHistoryCap),
			densities:  ring.NewFloatRing(exitHistoryCap),
			tracked:    types.NewCategory(types.CategoryTypeNone),
		}

		for _, value := range []float64{10, 10, 10, 10, 8, 6, 4, 2} {
			history.bidDepths.Push(value)
			history.spreads.Push(5)
			history.pressures.Push(0.8)
			history.imbalances.Push(0.6)
			history.densities.Push(4)
		}

		history.bidDepths.Push(1)
		history.spreads.Push(20)
		history.pressures.Push(0.1)
		history.imbalances.Push(-0.4)

		measurement, standout, err := exhaustMeasurement(history, history.tracked)

		Convey("It should emit an exhaustion measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, types.SourceExhaustion)
			So(measurement.Category, ShouldNotEqual, types.CategoryTypeNone)
			So(standout, ShouldBeGreaterThanOrEqualTo, 0)
			So(measurement.Strength, ShouldAlmostEqual, standout, 1e-12)
			So(measurement.Strength, ShouldBeLessThan, 1)
		})
	})
}
