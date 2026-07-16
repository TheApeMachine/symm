package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestResonanceUpdate(t *testing.T) {
	Convey("Given a chronological sequence of finite manifold states", t, func() {
		resonance := NewResonance(
			"BTC/USD", manifold.DefaultBaselineHalflife(),
		)
		var measurements []*types.Measurement
		producedAt := 0

		for index := 1; index <= 16 && len(measurements) == 0; index++ {
			state := causalState(
				time.Unix(int64(index), 0),
				100+float64(index),
				uint64(index),
			)
			state.Reading.PressureGradX += float64(index) / 100
			measurements, _ = resonance.Update(state)
			producedAt = index
		}

		Convey("It should return numerical resonance evidence without a category", func() {
			So(measurements, ShouldHaveLength, 2)
			So(measurements[0].Source, ShouldEqual, types.SourceResonance)
			So(measurements[0].Stream, ShouldEqual, types.Resonance)
			So(measurements[0].Metric, ShouldEqual, types.MetricResonanceEnergy)
			So(measurements[1].Metric, ShouldEqual, types.MetricResonanceSurprise)
		})

		Convey("Its maturity should grow from observed event-time coverage", func() {
			nextIndex := producedAt + 1
			state := causalState(
				time.Unix(int64(nextIndex), 0),
				100+float64(nextIndex),
				uint64(nextIndex),
			)
			state.Reading.PressureGradX += float64(nextIndex) / 100
			next, outcome := resonance.Update(state)

			So(next, ShouldHaveLength, 2)
			So(outcome, ShouldNotBeNil)
			So(next[0].Maturity, ShouldBeGreaterThan, measurements[0].Maturity)
			So(next[0].Maturity, ShouldBeLessThan, 1)
			So(outcome.Target, ShouldEqual, resonanceReturnTarget)
			So(outcome.ReturnReady, ShouldBeTrue)
			So(outcome.CalibrationSamples, ShouldBeGreaterThan, 0)
			So(outcome.IncrementalMSE, ShouldBeGreaterThanOrEqualTo, 0)
			So(outcome.Uncertainty, ShouldBeGreaterThanOrEqualTo, 0)
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

			So(next, ShouldHaveLength, 2)
			So(next[0].Horizon, ShouldEqual, time.Duration(0))
			So(next[1].Horizon, ShouldEqual, time.Duration(0))
		})
	})
}

func TestResonanceUpdateUsesConfiguredHalflife(t *testing.T) {
	Convey("Given a resonance model with a two-second baseline halflife", t, func() {
		resonance := NewResonance("BTC/USD", 2*time.Second)
		resonance.Update(causalState(time.Unix(1, 0), 100, 1))

		measurements, _ := resonance.Update(
			causalState(time.Unix(3, 0), 101, 2),
		)

		Convey("Its maturity should reach one half after one configured halflife", func() {
			So(measurements, ShouldHaveLength, 2)
			So(measurements[0].Maturity, ShouldAlmostEqual, 0.5)
		})
	})
}

func BenchmarkResonanceUpdate(b *testing.B) {
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
