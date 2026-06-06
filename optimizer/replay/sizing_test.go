package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestEntryDeployFraction(t *testing.T) {
	Convey("Given a node multiplier and choppy regime", t, func() {
		key := "trading.replay.choppy_size_scale"
		original := viper.Get(key)
		viper.Set(key, 0.5)
		defer viper.Set(key, original)

		costs := ReplayCosts{PositionFraction: 0.1}
		act := reasoning.Act{Fraction: 2}
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
		}

		fraction := entryDeployFraction(costs, act, snapshots)

		Convey("It should scale the base fraction by the node multiplier", func() {
			So(fraction, ShouldAlmostEqual, 0.2, 1e-9)
		})
	})
}
