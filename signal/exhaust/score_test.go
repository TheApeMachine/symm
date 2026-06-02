package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/ring"
)

func TestExitScoreLong(t *testing.T) {
	Convey("Given deteriorating long-side book history", t, func() {
		history := symbolHistory{
			bidDepths:  ring.FloatRing{},
			spreads:    ring.FloatRing{},
			pressures:  ring.FloatRing{},
			imbalances: ring.FloatRing{},
			densities:  ring.FloatRing{},
		}

		for _, value := range []float64{10, 10, 10, 10, 8, 6} {
			history.bidDepths.Push(value)
			history.spreads.Push(4)
			history.pressures.Push(0.7)
			history.imbalances.Push(0.5)
			history.densities.Push(3)
		}

		history.bidDepths.Push(1)
		history.spreads.Push(15)
		history.pressures.Push(0.05)
		history.imbalances.Push(-0.6)

		urgency, category, evidence := exitScoreLong(history)

		Convey("It should score positive exit urgency", func() {
			So(urgency, ShouldBeGreaterThan, 0)
			So(category, ShouldNotEqual, perspectives.CategoryTypeNone)
			So(evidence, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}
