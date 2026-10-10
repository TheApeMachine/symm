package data

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestScaledStandardization(t *testing.T) {
	gaps := []float64{1, 2, 0.5, 3, 1, 2, 1, 0.7, 1.5, 2}

	Convey("Given a per-trade rate stream whose next gap is ten microseconds", t, func() {
		state := &standardizer{}

		for _, gap := range gaps {
			metric := NewMetric("trade_rate", 1/gap, UnitTradeRate, TimescaleRollingWindow)
			So(metric.finalize(state), ShouldBeNil)
		}

		metric := NewMetric("trade_rate", 1/10e-6, UnitTradeRate, TimescaleRollingWindow)
		So(metric.finalize(state), ShouldBeNil)

		Convey("the z-score is taken on ln(rate), not the raw rate", func() {
			logs := make([]float64, len(gaps))

			for idx, gap := range gaps {
				logs[idx] = math.Log(1 / gap)
			}

			mean, m2 := 0.0, 0.0

			for _, value := range logs {
				mean += value
			}

			mean /= float64(len(logs))

			for _, value := range logs {
				m2 += (value - mean) * (value - mean)
			}

			want := (math.Log(1/10e-6) - mean) / math.Sqrt(m2/float64(len(logs)-1))
			So(metric.Standardizable(), ShouldBeTrue)
			So(metric.Standardized, ShouldAlmostEqual, want, 1e-9)
			// Linearly this gap scored about 1.9e5 sigma.
			So(metric.Standardized, ShouldBeLessThan, 100)
		})
	})

	Convey("Given a log-scaled stream that observes a non-positive value", t, func() {
		state := &standardizer{}

		for _, gap := range gaps {
			So(NewMetric("touch_fill_rate:bid", 1/gap, UnitRate, TimescaleRollingWindow).finalize(state), ShouldBeNil)
		}

		count := state.count
		metric := NewMetric("touch_fill_rate:bid", 0, UnitRate, TimescaleRollingWindow)
		So(metric.finalize(state), ShouldBeNil)

		Convey("its z-score is undefined and it stays out of the stream", func() {
			So(metric.Standardizable(), ShouldBeFalse)
			So(metric.Standardized, ShouldEqual, 0)
			So(state.count, ShouldEqual, count)
		})
	})

	Convey("Given a signed velocity stream on the log-modulus scale", t, func() {
		state := &standardizer{}
		values := []float64{1, -2, 0.5, -3, 1, 2, -1, 0.7, -1.5, 2, 0.25}
		var last *Metric

		for _, value := range values {
			last = NewMetric("gross_notional_rate_velocity", value, UnitVelocity, TimescaleRollingWindow)
			So(last.finalize(state), ShouldBeNil)
		}

		Convey("the first value only sets the unit and the sign survives", func() {
			So(state.count, ShouldEqual, len(values)-1)

			huge := NewMetric("gross_notional_rate_velocity", -1e9, UnitVelocity, TimescaleRollingWindow)
			So(huge.finalize(state), ShouldBeNil)
			So(huge.Standardizable(), ShouldBeTrue)
			So(huge.Standardized, ShouldBeLessThan, 0)
			// Linearly this velocity scored about -6e8 sigma.
			So(math.Abs(huge.Standardized), ShouldBeLessThan, 100)

			zero := NewMetric("gross_notional_rate_velocity", 0, UnitVelocity, TimescaleRollingWindow)
			So(zero.finalize(state), ShouldBeNil)
			So(zero.Standardizable(), ShouldBeTrue)
		})
	})

	Convey("Given the scale declarations", t, func() {
		So(CanonicalScale("trade_rate"), ShouldEqual, ScaleLog)
		So(CanonicalScale("excitation_amplitude:buy_from_sell"), ShouldEqual, ScaleLog)
		So(CanonicalScale("divergence_velocity:bid"), ShouldEqual, ScaleLogModulus)
		So(CanonicalScale("depth_zscore:bid"), ShouldEqual, ScaleLinear)
		So(core.MinimumPrior, ShouldEqual, 9)
	})
}
