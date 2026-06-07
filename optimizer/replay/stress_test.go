package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestExecutionStressMultiplier(t *testing.T) {
	convey.Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := executionStressMultiplier(snapshots)

		convey.Convey("It should expand slippage above baseline", func() {
			convey.So(multiplier, convey.ShouldBeGreaterThan, 1)
		})
	})

	convey.Convey("Given only laminar readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryLaminar, SNR: 2},
		}

		convey.Convey("It should leave slippage unchanged", func() {
			convey.So(executionStressMultiplier(snapshots), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given toxic microstructure readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryToxicBluff, SNR: 3},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := executionStressMultiplier(snapshots)

		convey.Convey("It should expand slippage from toxicity stress", func() {
			convey.So(multiplier, convey.ShouldBeGreaterThan, 1)
		})
	})
}

func TestRegimeHostility(t *testing.T) {
	convey.Convey("Adverse selection is anchored to the structural regime", t, func() {
		convey.So(broker.RegimeHostility(types.RegimeBearish), convey.ShouldBeGreaterThan, 1)
		convey.So(broker.RegimeHostility(types.RegimeChoppy), convey.ShouldBeGreaterThan, 1)

		convey.Convey("A liquidation/bearish regime is the most hostile", func() {
			convey.So(
				broker.RegimeHostility(types.RegimeBearish),
				convey.ShouldBeGreaterThan,
				broker.RegimeHostility(types.RegimeChoppy),
			)
		})

		convey.Convey("Calm and trending regimes are neutral", func() {
			convey.So(broker.RegimeHostility(types.RegimeTrending), convey.ShouldEqual, 1)
			convey.So(broker.RegimeHostility(types.RegimeBullish), convey.ShouldEqual, 1)
			convey.So(broker.RegimeHostility(types.RegimeDead), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkExecutionStressMultiplier(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
		{Category: types.CategoryLaminar, SNR: 0.5},
	}

	for b.Loop() {
		_ = executionStressMultiplier(snapshots)
	}
}

func BenchmarkRegimeHostility(b *testing.B) {
	for b.Loop() {
		_ = broker.RegimeHostility(types.RegimeBearish)
	}
}
