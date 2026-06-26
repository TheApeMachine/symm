package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/statutil"
)

func TestFieldConfigDerivations(t *testing.T) {
	Convey("Given book depth and integration cadence (universe size unknown)", t, func() {
		viper.Set("market.book_depth_levels", 8)
		viper.Set("signals.manifold.integration_interval", "250ms")

		Convey("Physics timing and tick size depend on depth alone", func() {
			So(integrationDeltaT(250*time.Millisecond, 8), ShouldAlmostEqual, 0.25, 0.0001)
			So(fieldTickSize(8), ShouldAlmostEqual, 1.0/(1<<8), 1e-12)
			So(fieldMeasurementsCapacity(250*time.Millisecond), ShouldEqual,
				statutil.SampleBudgetFromCadence(0.25))
		})

		Convey("Grid Y is the lane count, not a symbol count", func() {
			So(LaneCount, ShouldEqual, 3)
		})
	})
}
