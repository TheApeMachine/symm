package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestResonanceUpdate(t *testing.T) {
	withResonanceRLS(t)

	Convey("Given a chronological sequence of finite manifold states", t, func() {
		resonance := NewResonance(
			"BTC/USD", manifold.DefaultBaselineHalflife(),
		)
		var measurements []*types.Measurement
		producedAt := 0

		for index := 1; index <= 32; index++ {
			state := causalState(
				time.Unix(int64(index), 0),
				100+float64(index),
				uint64(index),
			)
			state.Reading.PressureGradX += float64(index) / 100
			next, _ := resonance.Update(state)
			producedAt = index

			if len(next) == 0 {
				continue
			}

			energy, ok := next[0].Sample(
				types.MetricResonanceEnergy, types.SideNone,
			)

			if !ok || energy.Raw <= 0 {
				continue
			}

			measurements = next
			break
		}

		Convey("It should return numerical resonance evidence without a category", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Source, ShouldEqual, types.SourceResonance)

			energy, ok := measurements[0].Sample(
				types.MetricResonanceEnergy, types.SideNone,
			)
			So(ok, ShouldBeTrue)
			So(energy.Raw, ShouldBeGreaterThan, 0)

			surprise, ok := measurements[0].Sample(
				types.MetricResonanceSurprise, types.SideNone,
			)
			So(ok, ShouldBeTrue)
			So(surprise.Raw, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("Its maturity should grow from observed event-time coverage", func() {
			var next []*types.Measurement
			var outcome *ResonanceOutcome
			readyAt := 0

			for nextIndex := producedAt + 1; nextIndex <= producedAt+32; nextIndex++ {
				state := causalState(
					time.Unix(int64(nextIndex), 0),
					100+float64(nextIndex),
					uint64(nextIndex),
				)
				state.Reading.PressureGradX += float64(nextIndex) / 100
				next, outcome = resonance.Update(state)

				if outcome != nil && outcome.ReturnReady {
					readyAt = nextIndex
					break
				}
			}

			So(next, ShouldHaveLength, 1)
			So(outcome, ShouldNotBeNil)
			So(next[0].Maturity, ShouldBeGreaterThan, measurements[0].Maturity)
			So(next[0].Maturity, ShouldBeLessThan, 1)
			So(outcome.Target, ShouldEqual, resonanceReturnTarget)
			So(outcome.ReturnReady, ShouldBeTrue)
			So(outcome.CalibrationSamples, ShouldBeGreaterThan, 0)
			So(outcome.IncrementalSkillLowerBound, ShouldBeGreaterThan, 0)
			So(outcome.IncrementalMSE, ShouldBeGreaterThanOrEqualTo, 0)
			So(outcome.Uncertainty, ShouldBeGreaterThanOrEqualTo, 0)

			for nextIndex := readyAt + 1; nextIndex <= readyAt+16; nextIndex++ {
				state := causalState(
					time.Unix(int64(nextIndex), 0),
					100+float64(nextIndex),
					uint64(nextIndex),
				)
				state.Reading.PressureGradX += float64(nextIndex) / 100
				_, outcome = resonance.Update(state)

				if outcome != nil {
					So(outcome.ReturnReady, ShouldBeTrue)
				}
			}
		})

		Convey("It should not publish a negative observation horizon", func() {
			nextIndex := producedAt + 1
			state := causalState(
				time.Unix(int64(nextIndex), 0),
				100+float64(nextIndex),
				uint64(nextIndex),
			)
			state.Duration = -time.Second
			state.Reading.PressureGradX += float64(nextIndex) / 100
			next, _ := resonance.Update(state)

			So(next, ShouldHaveLength, 1)
			So(next[0].Horizon, ShouldEqual, time.Duration(0))
		})
	})
}

func TestResonanceUpdateUsesConfiguredHalflife(t *testing.T) {
	withResonanceRLS(t)

	Convey("Given a resonance model with a two-second baseline halflife", t, func() {
		resonance := NewResonance("BTC/USD", 2*time.Second)
		resonance.Update(causalState(time.Unix(1, 0), 100, 1))

		measurements, _ := resonance.Update(
			causalState(time.Unix(3, 0), 101, 2),
		)

		Convey("Its maturity should reach one half after one configured halflife", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Maturity, ShouldAlmostEqual, 0.5)
		})
	})
}

func BenchmarkResonanceUpdate(b *testing.B) {
	withResonanceRLS(b)
	resonance := NewResonance(
		"BTC/USD", manifold.DefaultBaselineHalflife(),
	)
	b.ReportAllocs()

	for index := 1; b.Loop(); index++ {
		state := causalState(
			time.Unix(int64(index), 0),
			100+float64(index),
			uint64(index),
		)
		state.Reading.PressureGradX += float64(index) / 100
		resonance.Update(state)
	}
}

/*
withResonanceRLS pins the documented production return-head configuration and
restores the process-wide test configuration afterward.
*/
func withResonanceRLS(t testing.TB) {
	t.Helper()
	previousVariance := viper.Get("market.forecast.rls.initial_variance")
	previousForgetting := viper.Get("market.forecast.rls.forgetting_factor")
	previousConfidence := viper.Get("market.forecast.rls.calibration_confidence")
	viper.Set("market.forecast.rls.initial_variance", 1.0)
	viper.Set("market.forecast.rls.forgetting_factor", 1.0)
	viper.Set("market.forecast.rls.calibration_confidence", 0.95)
	t.Cleanup(func() {
		viper.Set("market.forecast.rls.initial_variance", previousVariance)
		viper.Set("market.forecast.rls.forgetting_factor", previousForgetting)
		viper.Set("market.forecast.rls.calibration_confidence", previousConfidence)
	})
}
