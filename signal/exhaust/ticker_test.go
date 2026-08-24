package exhaust

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/theapemachine/symm/kraken"
)

func ticker(symbol string, bid, ask float64, bidQty, askQty float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		Ask:       decimal.NewFromFloat64(ask),
		BidQty:    bidQty,
		AskQty:    askQty,
		Timestamp: at,
	}
}

func TestTickerStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)
	laterStill := time.Unix(1_700_000_002, 0)

	Convey("Given a valid touch snapshot", t, func() {
		entity := NewTicker()

		Convey("Step produces exactly one measurement with no warmup", func() {
			measurement := entity.Step(ticker("BTC/USD", 99, 101, 2, 2, now))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["displayed_depth_notional:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(measurement.Metrics["displayed_depth_notional:ask"].Raw, ShouldAlmostEqual, 202.0, 1e-12)
			So(measurement.Metrics["displayed_depth_notional"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(measurement.Metrics["spread"].Raw, ShouldAlmostEqual, 2.0, 1e-12)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)
			So(measurement.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["previous_book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["book_imbalance_change"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
			So(measurement.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, 0.0, 1e-12)

			So(measurement.Maturity, ShouldEqual, 0.0)

			// Log-space baselines are seeded with the first value, so each ratio
			// is one; divergence, z-score, and velocity need a prior observation.
			So(measurement.Metrics["book_imbalance_baseline"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["depth_baseline:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(measurement.Metrics["depth_baseline:ask"].Raw, ShouldAlmostEqual, 202.0, 1e-12)
			So(measurement.Metrics["depth_ratio:bid"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["depth_ratio:ask"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["total_depth_baseline"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(measurement.Metrics["total_depth_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["relative_spread_baseline"].Raw, ShouldAlmostEqual, 0.02, 1e-12)
			So(measurement.Metrics["spread_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-12)

			So(measurement.Metrics, ShouldNotContainKey, "book_imbalance_zscore")
			So(measurement.Metrics, ShouldNotContainKey, "book_imbalance_velocity")
			So(measurement.Metrics, ShouldNotContainKey, "depth_divergence:bid")
			So(measurement.Metrics, ShouldNotContainKey, "depth_zscore:bid")
			So(measurement.Metrics, ShouldNotContainKey, "depth_divergence_velocity:bid")
			So(measurement.Metrics, ShouldNotContainKey, "depth_divergence:ask")
			So(measurement.Metrics, ShouldNotContainKey, "depth_zscore:ask")
			So(measurement.Metrics, ShouldNotContainKey, "depth_divergence_velocity:ask")
			So(measurement.Metrics, ShouldNotContainKey, "total_depth_zscore")
			So(measurement.Metrics, ShouldNotContainKey, "spread_divergence")
			So(measurement.Metrics, ShouldNotContainKey, "spread_zscore")
			So(measurement.Metrics, ShouldNotContainKey, "spread_divergence_velocity")
		})

		Convey("stateful metrics advance over a multi-update sequence", func() {
			first := entity.Step(ticker("BTC/USD", 99, 101, 2, 2, now))
			So(first.Err, ShouldBeNil)

			second := entity.Step(ticker("BTC/USD", 100, 102, 4, 2, later))

			So(second, ShouldNotBeNil)
			So(second.Err, ShouldBeNil)

			So(second.Metrics["displayed_depth_notional:bid"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(second.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, 196.0/604.0, 1e-12)
			So(second.Metrics["previous_book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(second.Metrics["book_imbalance_change"].Raw, ShouldAlmostEqual, 196.0/604.0+0.01, 1e-12)
			So(second.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)

			So(second.Maturity, ShouldEqual, 0.5)
			So(second.SNR, ShouldBeGreaterThan, 0.0)

			// Estimator chains now emit divergence, z-score, and velocity.
			So(second.Metrics["book_imbalance_velocity"].Raw, ShouldAlmostEqual, 196.0/604.0+0.01, 1e-12)
			So(second.Metrics, ShouldContainKey, "book_imbalance_zscore")
			So(second.Metrics, ShouldContainKey, "depth_baseline:bid")
			So(second.Metrics, ShouldContainKey, "depth_divergence:bid")
			So(second.Metrics, ShouldContainKey, "depth_zscore:bid")
			So(second.Metrics, ShouldContainKey, "depth_ratio:bid")
			So(second.Metrics, ShouldContainKey, "depth_divergence:ask")
			So(second.Metrics, ShouldContainKey, "depth_zscore:ask")
			So(second.Metrics, ShouldContainKey, "total_depth_baseline")
			So(second.Metrics, ShouldContainKey, "total_depth_ratio")
			So(second.Metrics, ShouldContainKey, "total_depth_zscore")
			So(second.Metrics, ShouldContainKey, "relative_spread_baseline")
			So(second.Metrics, ShouldContainKey, "spread_divergence")
			So(second.Metrics, ShouldContainKey, "spread_zscore")
			So(second.Metrics, ShouldContainKey, "spread_ratio")

			// Divergence velocity needs two residuals and is still absent here.
			So(second.Metrics, ShouldNotContainKey, "depth_divergence_velocity:bid")
			So(second.Metrics, ShouldNotContainKey, "spread_divergence_velocity")

			// A third observation lets the divergence velocities appear.
			third := entity.Step(ticker("BTC/USD", 100, 102, 6, 2, laterStill))

			So(third, ShouldNotBeNil)
			So(third.Err, ShouldBeNil)
			So(third.Metrics, ShouldContainKey, "depth_divergence_velocity:bid")
			So(third.Metrics, ShouldContainKey, "depth_divergence_velocity:ask")
			So(third.Metrics, ShouldContainKey, "spread_divergence_velocity")
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(ticker("BTC/USD", 101, 99, 1, 1, now))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
