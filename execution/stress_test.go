package execution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestStressMultiplier(t *testing.T) {
	testconfig.Load(t)

	Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := StressMultiplier(snapshots)

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})

	Convey("Given hostile SNR in a bearish regime", t, func() {
		perspectives.PublishRegime(types.RegimeBearish)

		multiplier := StressFromHostileSNR(2, types.RegimeBearish)

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})
}

func BenchmarkStressMultiplier(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
		{Category: types.CategoryLaminar, SNR: 0.5},
	}

	for b.Loop() {
		_ = StressMultiplier(snapshots)
	}
}
