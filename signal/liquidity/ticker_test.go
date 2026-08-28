package liquidity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func tickerAt(symbol string, bid, ask float64, bidQty, askQty float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		Ask:       decimal.NewFromFloat64(ask),
		BidQty:    bidQty,
		AskQty:    askQty,
		Timestamp: at,
	}
}

func ticker(symbol string, bid, ask, bidQty, askQty float64) kraken.TickerData {
	return tickerAt(symbol, bid, ask, bidQty, askQty, time.Unix(1_700_000_000, 0))
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid touch snapshot", t, func() {
		entity := NewTicker()

		Convey("Step produces one measurement with correct direct touch facts", func() {
			measurement := entity.Step(ticker("BTC/USD", 99, 101, 1.0, 1.0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)
			So(measurement.Metrics["touch_notional:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["touch_notional:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_notional_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["two_sided_touch_notional"].Raw, ShouldEqual, 99.0)
		})

		Convey("the historical family is undefined on the first observation", func() {
			measurement := entity.Step(ticker("BTC/USD", 99, 101, 1.0, 1.0))

			_, hasDivergence := measurement.Metrics["depth_divergence:bid"]
			_, hasNoise := measurement.Metrics["depth_noise_scale:bid"]
			_, hasZScore := measurement.Metrics["depth_zscore:bid"]
			_, hasVelocity := measurement.Metrics["divergence_velocity:bid"]
			_, hasSpreadDivergence := measurement.Metrics["spread_divergence"]
			_, hasSpreadZScore := measurement.Metrics["spread_zscore"]

			So(hasDivergence, ShouldBeFalse)
			So(hasNoise, ShouldBeFalse)
			So(hasZScore, ShouldBeFalse)
			So(hasVelocity, ShouldBeFalse)
			So(hasSpreadDivergence, ShouldBeFalse)
			So(hasSpreadZScore, ShouldBeFalse)

			// No causal baseline yet: estimator support is one, so maturity is
			// zero, and there is no noise model to define a scalar SNR.
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(ticker("BTC/USD", 101, 99, 1.0, 1.0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

func TestTickerStepHistoricalFamily(t *testing.T) {
	Convey("Given a two-step depth sequence", t, func() {
		entity := NewTicker()
		base := time.Unix(1_700_000_000, 0)

		// First observation seeds the causal baseline (bid notional 100);
		// the second is measured against it (bid notional 200).
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("the divergence is the pre-observation log ratio", func() {
			bidDivergence, hasBidDivergence := second.Metrics["depth_divergence:bid"]
			So(hasBidDivergence, ShouldBeTrue)

			// depth_divergence:bid = log(200 / 100) = log(2), evaluated against
			// the causal pre-observation baseline (100), not the adapted one.
			So(bidDivergence.Raw, ShouldAlmostEqual, 0.6931471805599453, 1e-9)
		})

		Convey("the published baseline is the adapted causal reference", func() {
			bidBaseline, hasBidBaseline := second.Metrics["touch_notional_baseline:bid"]
			So(hasBidBaseline, ShouldBeTrue)

			// The baseline adapts to the geometric mean of the retained depth:
			// sqrt(100 * 200).
			So(bidBaseline.Raw, ShouldAlmostEqual, 141.4213562373095, 1e-9)
		})

		Convey("the depth ratio is current over the adapted baseline", func() {
			bidRatio, hasBidRatio := second.Metrics["depth_ratio:bid"]
			So(hasBidRatio, ShouldBeTrue)

			// 200 / sqrt(100*200) = sqrt(2).
			So(bidRatio.Raw, ShouldAlmostEqual, 1.4142135623730951, 1e-9)
		})

		Convey("the z-score is the divergence standardized by the residual scale", func() {
			bidZScore, hasBidZScore := second.Metrics["depth_zscore:bid"]
			So(hasBidZScore, ShouldBeTrue)

			// With one residual, dispersion = |log 2|, so z = 1.
			So(bidZScore.Raw, ShouldAlmostEqual, 1.0, 1e-9)

			bidNoise, hasBidNoise := second.Metrics["depth_noise_scale:bid"]
			So(hasBidNoise, ShouldBeTrue)
			So(bidNoise.Raw, ShouldAlmostEqual, 0.6931471805599453, 1e-9)
		})
	})
}

func TestTickerMorphologyOwnership(t *testing.T) {
	Convey("Given the liquidity projection", t, func() {
		entity := NewTicker()
		measurement := entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, time.Unix(1_700_000_000, 0)))

		Convey("liquidity does not duplicate morphology's structural geometry", func() {
			// Full-book shape/concentration/entropy belong to signal/morphology,
			// not to the touch-capacity Liquidity signal (METRIC_MAP §8).
			for _, forbidden := range []string{
				"book_shape_distance",
				"book_shape_ks",
				"concentration:bid",
				"concentration:ask",
				"entropy:bid",
				"entropy:ask",
				"morphology_change",
			} {
				_, present := measurement.Metrics[forbidden]
				So(present, ShouldBeFalse)
			}
		})
	})
}

func TestTickerStepIrregularCadence(t *testing.T) {
	Convey("Given the same notional jump over different elapsed intervals", t, func() {
		base := time.Unix(1_700_000_000, 0)

		stepEntity := func(secondElapsed time.Duration) *data.Measurement[float64] {
			entity := NewTicker()

			// Seed, then advance twice with the same per-step notional jump so
			// the divergence series has two observations to difference against.
			entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
			entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(secondElapsed)))
			return entity.Step(tickerAt("BTC/USD", 100, 102, 3.0, 1.0, base.Add(2*secondElapsed)))
		}

		fast := stepEntity(time.Second)
		slow := stepEntity(2 * time.Second)

		Convey("the velocity is a rate per second, not a per-message delta", func() {
			fastVelocity, hasFast := fast.Metrics["divergence_velocity:bid"]
			slowVelocity, hasSlow := slow.Metrics["divergence_velocity:bid"]

			So(hasFast, ShouldBeTrue)
			So(hasSlow, ShouldBeTrue)

			// Same divergence delta, but the slow run took twice the elapsed
			// time, so its rate is half the fast run's.
			So(fastVelocity.Raw, ShouldAlmostEqual, 2.0*slowVelocity.Raw, 1e-9)
		})
	})
}
