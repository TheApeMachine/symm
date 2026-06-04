package profile

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

func TestProfileIsInformativeGate(t *testing.T) {
	Convey("Given a category whose SNR readings pile at the zero floor", t, func() {
		profile := Profile{ctx: context.Background()}
		// Two clamped-to-zero readings plus two real spikes — the shape SNR
		// actually produces (snr.go clamps below-baseline readings to 0).
		for _, snr := range []float64{0, 0, 1, 3} {
			profile.Add(perspectives.Measurement{
				Category: perspectives.CategoryLaminar,
				SNR:      snr,
			})
		}
		profile.PrepareCache()

		informative := func(condition perspectives.ConditionType, value float64) bool {
			return profile.IsInformativeGate(
				perspectives.CategoryLaminar, perspectives.UnitSNR, condition, value,
			)
		}

		Convey("`snr >= 0` is vacuous (fires on every reading)", func() {
			So(informative(perspectives.ConditionIsGreaterThanOrEqual, 0), ShouldBeFalse)
		})

		Convey("`snr <= max` is vacuous (fires on every reading)", func() {
			So(informative(perspectives.ConditionIsLessThanOrEqual, 3), ShouldBeFalse)
		})

		Convey("`snr >= aboveFloor` discriminates and is informative", func() {
			So(informative(perspectives.ConditionIsGreaterThanOrEqual, 1), ShouldBeTrue)
		})

		Convey("`snr >= aboveMax` never fires and is not informative", func() {
			So(informative(perspectives.ConditionIsGreaterThanOrEqual, 5), ShouldBeFalse)
		})

		Convey("A gate over an unseen category is not informative", func() {
			So(profile.IsInformativeGate(
				perspectives.CategoryToxicBluff, perspectives.UnitSNR,
				perspectives.ConditionIsGreaterThanOrEqual, 1,
			), ShouldBeFalse)
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
