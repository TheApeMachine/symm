package analyze

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestDiagnosticThresholdsForRegime(t *testing.T) {
	Convey("Given choppy and dead regimes", t, func() {
		choppy := DiagnosticThresholdsForRegime(types.RegimeChoppy)
		dead := DiagnosticThresholdsForRegime(types.RegimeDead)
		base := DefaultDiagnosticThresholds()

		Convey("Choppy should tolerate higher flicker than dead", func() {
			So(choppy.FlickerCrossing, ShouldBeGreaterThan, dead.FlickerCrossing)
			So(choppy.FlickerCrossing, ShouldBeGreaterThan, base.FlickerCrossing)
			So(dead.FlickerCrossing, ShouldBeLessThan, base.FlickerCrossing)
		})
	})
}
