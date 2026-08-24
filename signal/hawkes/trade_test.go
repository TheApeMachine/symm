package hawkes

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

			So(measurement.Metrics["conditional_intensity"].Raw, ShouldAlmostEqual, 1.1, 1e-12)
			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["conditional_intensity:sell"].Raw, ShouldAlmostEqual, 0.1, 1e-12)

			So(measurement.Metrics["background_rate"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["background_rate:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["background_rate:sell"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["excitation_intensity:buy"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["excitation_intensity:sell"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			So(measurement.Metrics["excitation_fraction:buy"].Raw, ShouldEqual, 0.0)
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

			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldAlmostEqual, 1.1, 1e-12)
			So(measurement.Metrics["conditional_intensity:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["excitation_intensity:buy"].Raw, ShouldAlmostEqual, 0.1, 1e-12)
			So(measurement.Metrics["excitation_intensity:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["excitation_fraction:buy"].Raw, ShouldAlmostEqual, 0.1/1.1, 1e-12)
			So(measurement.Metrics["excitation_fraction:sell"].Raw, ShouldEqual, 0.0)

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
