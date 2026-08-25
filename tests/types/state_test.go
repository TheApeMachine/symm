package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	coretypes "github.com/theapemachine/symm/types"
)

func TestRegimeProfilePrecursorExpectation(t *testing.T) {
	Convey("Given the FastPump profile and its baseline", t, func() {
		profile := DefaultProfiles[FastPump]
		expectation := profile.PrecursorExpectation(DefaultProfiles[Baseline])

		Convey("It should declare a precursor distinguishable from baseline", func() {
			So(expectation.MinimumStepVolume,
				ShouldBeGreaterThan, expectation.MaximumBaselineStepVolume)
			So(expectation.MinimumBidQuantity,
				ShouldBeGreaterThan, expectation.MaximumBaselineBidQuantity)
			So(expectation.MaximumAskQuantity,
				ShouldBeLessThan, expectation.MinimumBaselineAskQuantity)
		})

		Convey("It should name its decision-facing Thesis evidence", func() {
			So(expectation.Contract.MinimumObservations,
				ShouldBeGreaterThan, 1)
			So(expectation.Contract.Metrics, ShouldContain,
				PrecursorMetricExpectation{
					Metric:            "volume_bar_notional",
					MinimumNormalized: PositiveEvidenceFloor,
				})
			So(expectation.Contract.Metrics, ShouldContain,
				PrecursorMetricExpectation{
					Metric:            "volume_bar_quantity",
					MinimumNormalized: NormalizedEmpiricalRatioBaseline,
				})
			So(expectation.Contract.Metrics, ShouldContain,
				PrecursorMetricExpectation{
					Metric:            "relative_spread",
					MinimumNormalized: PositiveEvidenceFloor,
				})
			So(expectation.Contract.Categories,
				ShouldContain, coretypes.VerticalIgnition)
			So(expectation.Contract.Categories,
				ShouldContain, coretypes.CoiledCompression)
		})
	})
}
