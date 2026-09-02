package sentiment

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func ticker(symbol string, last float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(last),
		Timestamp: at,
	}
}

func TestTickerStep(t *testing.T) {
	Convey("Given a shared cross-section", t, func() {
		entity := NewTicker()

		Convey("the first observation yields a measurement with no return yet", func() {
			measurement := entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldBeEmpty)
		})

		Convey("a quoted market with no recent trade remains outside the price cohort", func() {
			untraded := ticker("CORN/USD", 0, time.Unix(1_700_000_000, 0))
			untraded.Bid = decimal.NewFromFloat64(0.02015)
			untraded.Ask = decimal.NewFromFloat64(0.04414)

			measurement := entity.Step(untraded)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldBeEmpty)
			So(measurement.Metadata["support"], ShouldEqual, 0.0)
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Provenance["last_trade_price_state"], ShouldEqual, "unobserved")

			firstTrade := entity.Step(ticker("CORN/USD", 0.03, time.Unix(1_700_000_001, 0)))
			untraded.Timestamp = time.Unix(1_700_000_002, 0)
			unobservedAgain := entity.Step(untraded)
			secondTrade := entity.Step(ticker("CORN/USD", 0.033, time.Unix(1_700_000_003, 0)))

			So(firstTrade.Err, ShouldBeNil)
			So(firstTrade.Metrics, ShouldBeEmpty)
			So(unobservedAgain.Err, ShouldBeNil)
			So(unobservedAgain.Metrics, ShouldBeEmpty)
			So(secondTrade.Err, ShouldBeNil)
			So(secondTrade.Metrics["return"].Raw, ShouldAlmostEqual, math.Log(0.033/0.03), 1e-12)
		})

		Convey("a negative last price remains invalid", func() {
			measurement := entity.Step(ticker("AAA/USD", -1, time.Unix(1_700_000_000, 0)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("the second observation emits the member return and cohort facts", func() {
			entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))

			measurement := entity.Step(ticker("AAA/USD", 110, time.Unix(1_700_000_001, 0)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["return"].Raw, ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
			So(measurement.Metrics["absolute_return"].Raw, ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
			So(measurement.Metrics["cohort_member_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["valid_member_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["advance_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["decline_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["breadth"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["median_return"].Raw, ShouldAlmostEqual, 0.10, 1e-12)
			So(measurement.Metrics["median_absolute_return"].Raw, ShouldAlmostEqual, 0.10, 1e-12)
			So(measurement.Metrics["largest_absolute_return"].Raw, ShouldAlmostEqual, 0.10, 1e-12)
			So(measurement.Metrics["largest_move_tie_count"].Raw, ShouldEqual, 0.0)
		})

		Convey("a second symbol folds into the cohort and changes the facts", func() {
			entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))
			entity.Step(ticker("AAA/USD", 110, time.Unix(1_700_000_001, 0)))
			entity.Step(ticker("BBB/USD", 200, time.Unix(1_700_000_000, 0)))

			measurement := entity.Step(ticker("BBB/USD", 190, time.Unix(1_700_000_001, 0)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["return"].Raw, ShouldAlmostEqual, math.Log(190.0/200.0), 1e-12)
			So(measurement.Metrics["cohort_member_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["advance_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["decline_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["breadth"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["median_return"].Raw, ShouldAlmostEqual, 0.025, 1e-12)
			So(measurement.Metrics["median_absolute_return"].Raw, ShouldAlmostEqual, 0.075, 1e-12)
			So(measurement.Metrics["largest_absolute_return"].Raw, ShouldAlmostEqual, 0.10, 1e-12)
			So(measurement.Metrics["directional_agreement"].Raw, ShouldAlmostEqual, 0.5, 1e-12)
			So(measurement.Metrics["directional_consensus"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
			So(measurement.Metrics["opposite_direction_peer_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["opposite_direction_peer_fraction"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["excluded_member_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["largest_signed_return"].Raw, ShouldAlmostEqual, 0.10, 1e-12)
			So(measurement.Metrics["largest_move_share"].Raw, ShouldAlmostEqual, 0.10/0.15, 1e-12)
			So(measurement.Metrics["largest_move_ratio"].Raw, ShouldAlmostEqual, 0.10/0.075, 1e-12)
			So(measurement.Metrics["magnitude_mad"].Raw, ShouldAlmostEqual, 0.025, 1e-12)
			So(measurement.Metrics["asof_age_seconds"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["from_age_seconds"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Provenance["largest_move_symbol"], ShouldEqual, "AAA/USD")
		})

		Convey("a later cut exposes the causal estimator views", func() {
			entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))
			entity.Step(ticker("AAA/USD", 110, time.Unix(1_700_000_001, 0)))
			entity.Step(ticker("BBB/USD", 200, time.Unix(1_700_000_000, 0)))
			entity.Step(ticker("BBB/USD", 190, time.Unix(1_700_000_001, 0)))

			measurement := entity.Step(ticker("AAA/USD", 121, time.Unix(1_700_000_002, 0)))

			So(measurement.Err, ShouldBeNil)
			// The breadth baseline is seeded from the first cut, so the derived
			// estimator views for a later cut are non-zero and causal.
			So(measurement.Metrics["breadth_baseline"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["breadth_divergence"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["median_return_baseline"].Raw, ShouldNotEqual, 0.0)
		})

		Convey("the first cut reports no SNR, the breadth estimator having no history", func() {
			measurement := entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.SNRDefined, ShouldBeFalse)
		})

		Convey("a settled breadth estimator yields a defined SNR", func() {
			// Move the cohort's direction from cut to cut so breadth actually
			// varies, which is what gives its estimator a noise model to report.
			for step := range 12 {
				at := time.Unix(1_700_000_000+int64(step), 0)
				drift := float64(step % 3)

				entity.Step(ticker("AAA/USD", 100+drift, at))
				entity.Step(ticker("BBB/USD", 200-drift, at))
			}

			measurement := entity.Step(ticker("AAA/USD", 140, time.Unix(1_700_000_012, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func BenchmarkTickerStep(b *testing.B) {
	entity := NewTicker()
	step := int64(0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		step++
		entity.Step(ticker(
			"BTC/USD",
			100+float64(step%10),
			time.Unix(1_700_000_000+step, 0),
		))
	}
}
