package sentiment

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
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
		workspace := runtime.NewWorkspace(nil)
		entity := NewTicker(workspace)

		Convey("the first observation yields a measurement with no return yet", func() {
			measurement := entity.Step(ticker("AAA/USD", 100, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldBeEmpty)
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
	})
}
