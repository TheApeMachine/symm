package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/statutil"
)

func TestFieldConfigDerivations(t *testing.T) {
	Convey("Given symbol count and integration cadence", t, func() {
		viper.Set("market.default_symbols", []string{"BTC/USD", "ETH/USD", "SOL/USD"})
		viper.Set("market.book_depth_levels", 8)
		viper.Set("signals.manifold.integration_interval", "250ms")

		Convey("It should derive batch sizing from symbol count", func() {
			So(ManifoldBatchCapacity(), ShouldEqual,
				3*8*statutil.SampleBudgetFromCadence(0.25))
			So(ManifoldFlushInterval(), ShouldEqual, 750*time.Millisecond)
		})

		Convey("It should derive physics timing from cadence and tick depth", func() {
			So(integrationDeltaT(250*time.Millisecond, 8, 3), ShouldAlmostEqual, 0.25, 0.0001)
			So(fieldTickSize(8, 3), ShouldAlmostEqual, 1.0/(1<<24), 1e-18)
			So(fieldMeasurementsCapacity(250*time.Millisecond, 3), ShouldEqual,
				statutil.SampleBudgetFromCadence(0.25)*3)
		})
	})
}
