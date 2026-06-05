package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestEntryDeployFraction(t *testing.T) {
	Convey("Given a node multiplier and choppy regime", t, func() {
		viper.Set("trading.replay.choppy_size_scale", 0.5)
		defer viper.Set("trading.replay.choppy_size_scale", 0)

		costs := ReplayCosts{PositionFraction: 0.1}
		act := perspectives.Act{Fraction: 2}
		snapshots := []perspectives.Measurement{
			{Category: perspectives.CategoryTurbulent, SNR: 2},
		}

		fraction := entryDeployFraction(costs, act, snapshots)

		Convey("It should scale the base fraction by the node multiplier", func() {
			So(fraction, ShouldAlmostEqual, 0.2, 1e-9)
		})
	})
}
