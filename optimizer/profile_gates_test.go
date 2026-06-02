package optimizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestProfileGatePassCount(t *testing.T) {
	Convey("Given cached profile values", t, func() {
		profile := Profile{ctx: context.Background()}
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      1,
		})
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      3,
		})
		profile.PrepareCache()

		Convey("It should count values passing the gate", func() {
			passes := profile.GatePassCount(
				perspectives.CategoryLaminar,
				perspectives.UnitSNR,
				perspectives.ConditionIsGreaterThanOrEqual,
				2,
			)

			So(passes, ShouldEqual, 1)
		})

		Convey("It should score gate selectivity", func() {
			score := profile.GateSelectivityScore(
				perspectives.CategoryLaminar,
				perspectives.UnitSNR,
				perspectives.ConditionIsGreaterThanOrEqual,
				2,
			)

			So(score, ShouldBeGreaterThan, 0)
		})
	})
}

func TestCountPassingValues(t *testing.T) {
	Convey("Given sorted gate values", t, func() {
		values := []float64{1, 2, 3, 4}

		Convey("It should count LTE thresholds", func() {
			So(countPassingValues(
				values, 2,
				perspectives.ConditionIsLessThanOrEqual,
			), ShouldEqual, 2)
		})
	})
}
