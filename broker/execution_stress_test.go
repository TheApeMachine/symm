package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestExecutionStressMultiplier(t *testing.T) {
	Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := ExecutionStressMultiplier(snapshots)

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})

	Convey("Given hostile symbol stress in a bearish regime", t, func() {
		perspectives.PublishRegime(types.RegimeBearish)

		multiplier := ExecutionStressFromSymbol(SymbolStress{
			FluidCategory: types.CategoryTurbulent,
			FluidSNR:      2,
		})

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})
}


func BenchmarkExecutionStressMultiplier(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
		{Category: types.CategoryLaminar, SNR: 0.5},
	}

	for b.Loop() {
		_ = ExecutionStressMultiplier(snapshots)
	}
}
