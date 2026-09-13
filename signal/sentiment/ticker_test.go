package sentiment

import (
	"context"
	"maps"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's last price written.
Zero is an unobserved market; a negative price is an invalid one.
*/
var schema = new(Ticker).Register().Metrics

func tick(symbol string, price float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("sentiment", maps.Clone(schema))
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["last"] = m.Metrics["last"].Write(price)

	return m
}

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func drive(entity *Ticker, symbol string, prices []float64) []*data.Measurement[float64] {
	measurements := make([]*data.Measurement[float64], 0, len(prices))

	for index, price := range prices {
		measurements = append(measurements, entity.Step(tick(
			symbol, price, timestamp(int64(index)+1),
		)))
	}

	return measurements
}

func TestTickerStep(t *testing.T) {
	Convey("Given a sentiment ticker-path instrument", t, func() {
		entity := NewTicker(context.Background())

		Convey("the first observation yields a measurement with no return yet", func() {
			measurement := entity.Step(tick("AAA/USD", 100, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["return"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["absolute_return"].Raw, ShouldEqual, 0.0)
			So(measurement.Metadata[data.MetadataSupport], ShouldEqual, 1.0)
		})

		Convey("a quoted market with no recent trade remains outside the price cohort", func() {
			measurement := entity.Step(tick("CORN/USD", 0, timestamp(1)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["return"].Raw, ShouldEqual, 0.0)
			So(measurement.Metadata["support"], ShouldEqual, 0.0)
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Provenance["last_trade_price_state"], ShouldEqual, "unobserved")

			firstTrade := entity.Step(tick("CORN/USD", 0.03, timestamp(2)))
			unobservedAgain := entity.Step(tick("CORN/USD", 0, timestamp(3)))
			secondTrade := entity.Step(tick("CORN/USD", 0.033, timestamp(4)))

			So(firstTrade.Err, ShouldBeNil)
			So(firstTrade.Metrics["return"].Raw, ShouldEqual, 0.0)
			So(unobservedAgain.Err, ShouldBeNil)
			So(unobservedAgain.Metrics["return"].Raw, ShouldEqual, 0.0)
			So(secondTrade.Err, ShouldBeNil)
			So(secondTrade.Metrics["return"].Raw, ShouldAlmostEqual, math.Log(0.033/0.03), 1e-12)
		})

		Convey("a negative last price remains invalid", func() {
			measurement := entity.Step(tick("AAA/USD", -1, timestamp(1)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("the second observation emits the member return and cohort facts", func() {
			entity.Step(tick("AAA/USD", 100, timestamp(1)))

			measurement := entity.Step(tick("AAA/USD", 110, timestamp(2)))

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
			entity.Step(tick("AAA/USD", 100, timestamp(1)))
			entity.Step(tick("AAA/USD", 110, timestamp(2)))
			entity.Step(tick("BBB/USD", 200, timestamp(1)))

			measurement := entity.Step(tick("BBB/USD", 190, timestamp(2)))

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
			entity.Step(tick("AAA/USD", 100, timestamp(1)))
			entity.Step(tick("AAA/USD", 110, timestamp(2)))
			entity.Step(tick("BBB/USD", 200, timestamp(1)))
			entity.Step(tick("BBB/USD", 190, timestamp(2)))

			measurement := entity.Step(tick("AAA/USD", 121, timestamp(3)))

			So(measurement.Err, ShouldBeNil)
			// The breadth baseline is seeded from the first cut, so the derived
			// estimator views for a later cut are non-zero and causal.
			So(measurement.Metrics["breadth_baseline"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["breadth_divergence"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["median_return_baseline"].Raw, ShouldNotEqual, 0.0)
		})

		Convey("the first cut reports no SNR, the breadth estimator having no history", func() {
			measurement := entity.Step(tick("AAA/USD", 100, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.SNRDefined, ShouldBeFalse)
		})

		Convey("a settled breadth estimator yields a defined SNR", func() {
			// Move the cohort's direction from cut to cut so breadth actually
			// varies, which is what gives its estimator a noise model to report.
			for step := range 12 {
				at := timestamp(int64(step))

				entity.Step(tick("AAA/USD", 100+float64(step%3), at))
				entity.Step(tick("BBB/USD", 200-float64(step%3), at))
			}

			measurement := entity.Step(tick("AAA/USD", 140, timestamp(12)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("a timestamp regression never touches the path", func() {
			entity.Step(tick("AAA/USD", 100, timestamp(2)))

			measurement := entity.Step(tick("AAA/USD", 110, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["return"].Raw, ShouldEqual, 0.0)
			So(measurement.Metadata[data.MetadataSupport], ShouldEqual, 0.0)
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "last")
			So(measurement.Metrics, ShouldContainKey, "breadth")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

/*
BenchmarkTickerStep isolates the intrinsic cost of one sentiment Step on a
focal symbol folding into a shared cross-section.
*/
func BenchmarkTickerStep(b *testing.B) {
	entity := NewTicker(context.Background())
	step := int64(0)

	measurement := tick("BTC/USD", 100, timestamp(0))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		step++
		measurement.Metrics["last"] = measurement.Metrics["last"].Write(100 + float64(step%10))
		measurement.At = timestamp(step)
		entity.Step(measurement)
	}
}
