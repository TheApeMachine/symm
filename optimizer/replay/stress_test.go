package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestExecutionStressMultiplier(t *testing.T) {
	convey.Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []perspectives.Measurement{
			{Category: perspectives.CategoryTurbulent, SNR: 2},
			{Category: perspectives.CategoryLaminar, SNR: 0.5},
		}

		multiplier := executionStressMultiplier(snapshots)

		convey.Convey("It should expand slippage above baseline", func() {
			convey.So(multiplier, convey.ShouldBeGreaterThan, 1)
		})
	})

	convey.Convey("Given only laminar readings", t, func() {
		snapshots := []perspectives.Measurement{
			{Category: perspectives.CategoryLaminar, SNR: 2},
		}

		convey.Convey("It should leave slippage unchanged", func() {
			convey.So(executionStressMultiplier(snapshots), convey.ShouldEqual, 1)
		})
	})
}

func TestRegimeHostility(t *testing.T) {
	convey.Convey("Adverse selection is anchored to the structural regime", t, func() {
		convey.So(regimeHostility(perspectives.RegimeBearish), convey.ShouldBeGreaterThan, 1)
		convey.So(regimeHostility(perspectives.RegimeChoppy), convey.ShouldBeGreaterThan, 1)

		convey.Convey("A liquidation/bearish regime is the most hostile", func() {
			convey.So(
				regimeHostility(perspectives.RegimeBearish),
				convey.ShouldBeGreaterThan,
				regimeHostility(perspectives.RegimeChoppy),
			)
		})

		convey.Convey("Calm and trending regimes are neutral", func() {
			convey.So(regimeHostility(perspectives.RegimeTrending), convey.ShouldEqual, 1)
			convey.So(regimeHostility(perspectives.RegimeBullish), convey.ShouldEqual, 1)
			convey.So(regimeHostility(perspectives.RegimeDead), convey.ShouldEqual, 1)
		})
	})
}
