package hawkes

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func hawkesTrade(symbol string, side string, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(100),
		Qty:       1,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a fresh arrival-dynamics entity", t, func() {
		entity := NewTrade()

		Convey("the first buy event yields a measurement with no warmup gating", func() {
			measurement := entity.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["event_fraction:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_fraction:sell"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["arrival_rate"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["arrival_rate:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["arrival_rate:sell"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["conditional_intensity"].Raw, ShouldAlmostEqual, 0.3, 1e-12)
			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldAlmostEqual, 0.2, 1e-12)
			So(measurement.Metrics["conditional_intensity:sell"].Raw, ShouldAlmostEqual, 0.1, 1e-12)

			So(measurement.Metrics["background_rate"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["background_rate:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["background_rate:sell"].Raw, ShouldEqual, 0.0)

			// Buy intensity opens below its own just-set background rate (the
			// rate is the cumulative average including this very event, while
			// conditional_intensity is the fitted process value): excitation
			// is genuinely negative here, not clamped to zero.
			So(measurement.Metrics["excitation_intensity:buy"].Raw, ShouldAlmostEqual, -0.8, 1e-12)
			So(measurement.Metrics["excitation_intensity:sell"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			So(measurement.Metrics["excitation_fraction:buy"].Raw, ShouldAlmostEqual, -4.0, 1e-12)
			So(measurement.Metrics["excitation_fraction:sell"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["excitation_amplitude:buy_from_buy"].Raw, ShouldAlmostEqual, 0.2, 1e-12)
			So(measurement.Metrics["excitation_amplitude:buy_from_sell"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			So(measurement.Metrics["excitation_amplitude:sell_from_buy"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			So(measurement.Metrics["excitation_amplitude:sell_from_sell"].Raw, ShouldAlmostEqual, 0.2, 1e-12)

			So(measurement.Metrics["excitation_decay:buy_from_buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["excitation_timescale:buy_from_buy"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["offspring:buy_from_buy"].Raw, ShouldAlmostEqual, 0.2, 1e-12)
			So(measurement.Metrics["branching_spectral_radius"].Raw, ShouldAlmostEqual, 0.3, 1e-12)
			So(measurement.Metrics["expected_descendants_from_buy"].Raw, ShouldAlmostEqual, 0.9/0.63, 1e-9)
			So(measurement.Metrics["expected_descendants_from_sell"].Raw, ShouldAlmostEqual, 0.9/0.63, 1e-9)

			So(measurement.Metrics["log_likelihood:hawkes"].Raw, ShouldNotBeZeroValue)

			// Per-event likelihood over a single event equals the total.
			So(measurement.Metrics["log_likelihood_per_event:hawkes"].Raw, ShouldEqual, measurement.Metrics["log_likelihood:hawkes"].Raw)

			// The first event closes no interval to integrate, so compensator
			// and innovation metrics are undefined.
			_, hasCompensator := measurement.Metrics["compensator:buy"]
			_, hasInnovation := measurement.Metrics["count_innovation:buy"]

			So(hasCompensator, ShouldBeFalse)
			So(hasInnovation, ShouldBeFalse)

			// One retained event is a single effective observation: Maturity 0.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a second sell event advances the mark composition and intensities", func() {
			entity.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0)))
			measurement := entity.Step(hawkesTrade("BTC/USD", "sell", time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_fraction:buy"].Raw, ShouldEqual, 0.5)
			So(measurement.Metrics["event_fraction:sell"].Raw, ShouldEqual, 0.5)

			So(measurement.Metrics["arrival_rate"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["arrival_rate:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["arrival_rate:sell"].Raw, ShouldEqual, 1.0)

			sellIntensityEvent2 := 0.1*math.Exp(-1) + 0.2

			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldAlmostEqual, 1.1, 1e-12)
			So(measurement.Metrics["conditional_intensity:sell"].Raw, ShouldAlmostEqual, sellIntensityEvent2, 1e-9)
			So(measurement.Metrics["excitation_intensity:buy"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			// Sell intensity decayed most of the way back toward its own
			// just-set background rate before this event's jump, landing
			// below that rate: excitation is genuinely negative, not zero.
			So(measurement.Metrics["excitation_intensity:sell"].Raw, ShouldAlmostEqual, sellIntensityEvent2-1.0, 1e-9)
			So(measurement.Metrics["excitation_fraction:buy"].Raw, ShouldAlmostEqual, 0.1/1.1, 1e-12)
			So(measurement.Metrics["excitation_fraction:sell"].Raw, ShouldAlmostEqual, (sellIntensityEvent2-1.0)/sellIntensityEvent2, 1e-9)

			So(measurement.Metrics["conditional_intensity_velocity"].Raw, ShouldAlmostEqual, math.Log((1.1+sellIntensityEvent2)/0.3), 1e-9)
			So(measurement.Metrics["spectral_radius_velocity"].Raw, ShouldEqual, 0.0)

			// Compensator integrates the pre-arrival intensity over the closed
			// interval; innovations are the observed counts minus the integral.
			So(measurement.Metrics["compensator:buy"].Raw, ShouldAlmostEqual, 1+0.1*math.E, 1e-9)
			So(measurement.Metrics["compensator:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["count_innovation:buy"].Raw, ShouldAlmostEqual, -0.1*math.E, 1e-9)
			So(measurement.Metrics["count_innovation:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["standardized_innovation:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["standardized_innovation:buy"].Raw, ShouldBeLessThan, 0.0)

			So(measurement.Maturity, ShouldEqual, 0.5)
		})

		Convey("a third buy event keeps the counts causally ordered", func() {
			entity.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0)))
			entity.Step(hawkesTrade("BTC/USD", "sell", time.Unix(1001, 0)))
			measurement := entity.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1003, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["arrival_rate:buy"].Raw, ShouldAlmostEqual, 2.0/3.0, 1e-12)
			So(measurement.Metrics["arrival_rate:sell"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)

			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldAlmostEqual, 1.2135335283236613, 1e-9)
		})
	})

	Convey("Given sustained clustered arrivals", t, func() {
		entity := NewTrade()

		Convey("the fitted MLE excitation eventually overrides the fallback amplitudes", func() {
			base := time.Unix(1000, 0)
			var last *data.Measurement[float64]

			for index := 0; index < 400; index++ {
				at := base.Add(time.Duration(index) * 300 * time.Millisecond)
				last = entity.Step(hawkesTrade("BTC/USD", "buy", at))
				So(last.Err, ShouldBeNil)

				at = at.Add(120 * time.Millisecond)
				last = entity.Step(hawkesTrade("BTC/USD", "sell", at))
				So(last.Err, ShouldBeNil)
			}

			amplitude := last.Metrics["excitation_amplitude:buy_from_buy"].Raw
			decay := last.Metrics["excitation_decay:buy_from_buy"].Raw

			So(amplitude, ShouldNotAlmostEqual, 0.2, 1e-9)
			So(decay, ShouldNotAlmostEqual, 1.0, 1e-9)
		})
	})

	Convey("Given a regressing event time", t, func() {
		entity := NewTrade()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			entity.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0)))
			measurement := entity.Step(hawkesTrade("BTC/USD", "sell", time.Unix(999, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
