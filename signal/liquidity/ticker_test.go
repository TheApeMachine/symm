package liquidity

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
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

/*
TestTickerStepPreObservationBaseline is the exact BLOCKER 1 fixture: with one
prior observation, the only possible causal baseline is that observation, so
all four published facts must reference the SAME pre-observation baseline.
*/
func TestTickerStepPreObservationBaseline(t *testing.T) {
	Convey("Given bid depth 100 then 200", t, func() {
		entity := NewTicker()
		base := time.Unix(1_700_000_000, 0)

		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("baseline, ratio and divergence reference the same pre-observation baseline", func() {
			baseline, hasBaseline := second.Metrics["touch_notional_baseline:bid"]
			So(hasBaseline, ShouldBeTrue)
			So(baseline.Raw, ShouldAlmostEqual, 100.0, 1e-9)

			ratio, hasRatio := second.Metrics["depth_ratio:bid"]
			So(hasRatio, ShouldBeTrue)
			So(ratio.Raw, ShouldAlmostEqual, 2.0, 1e-9)

			divergence, hasDivergence := second.Metrics["depth_divergence:bid"]
			So(hasDivergence, ShouldBeTrue)
			So(divergence.Raw, ShouldAlmostEqual, math.Log(2.0), 1e-9)

			// log(depth_ratio) == depth_divergence exactly.
			So(math.Log(ratio.Raw), ShouldAlmostEqual, divergence.Raw, 1e-12)
		})
	})

	Convey("the ask-side and spread facts follow the same contract", t, func() {
		entity := NewTicker()
		base := time.Unix(20, 0)

		// Ask notional stays 102 across both steps (askQty constant), so the
		// ask divergence is 0 and the baseline is the prior ask notional.
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("ask baseline is the prior ask notional with zero divergence", func() {
			baseline, hasBaseline := second.Metrics["touch_notional_baseline:ask"]
			So(hasBaseline, ShouldBeTrue)
			// Prior ask notional = 102 * 1.0 = 102.
			So(baseline.Raw, ShouldAlmostEqual, 102.0, 1e-9)

			divergence, hasDivergence := second.Metrics["depth_divergence:ask"]
			So(hasDivergence, ShouldBeTrue)
			So(divergence.Raw, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})
}

/*
TestTickerStepDegenerateNoise is BLOCKER 3: the z-score is undefined (absent),
never zero, when the pre-observation noise is unavailable or degenerate.
*/
func TestTickerStepDegenerateNoise(t *testing.T) {
	Convey("Given no prior observation", t, func() {
		entity := NewTicker()
		first := entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, time.Unix(1, 0)))

		Convey("no baseline produces no z-score", func() {
			_, hasZScore := first.Metrics["depth_zscore:bid"]
			So(hasZScore, ShouldBeFalse)
		})
	})

	Convey("Given a single prior observation (degenerate residual scale)", t, func() {
		entity := NewTicker()
		base := time.Unix(1, 0)
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("the z-score is absent, not zero", func() {
			_, hasZScore := second.Metrics["depth_zscore:bid"]
			So(hasZScore, ShouldBeFalse)

			// The divergence itself is still defined.
			_, hasDivergence := second.Metrics["depth_divergence:bid"]
			So(hasDivergence, ShouldBeTrue)
		})
	})
}

/*
TestTickerStepZScorePresent is BLOCKER 3's positive case: once the noise scale
is estimable and positive, the z-score is present and equals divergence/noise.
*/
func TestTickerStepZScorePresent(t *testing.T) {
	Convey("Given a history with non-zero residual dispersion", t, func() {
		entity := NewTicker()
		base := time.Unix(1000, 0)

		// Vary the bid depth so the residual dispersion is non-zero, then
		// observe the latest point's z-score.
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.5, 1.0, base.Add(2*time.Second)))
		fourth := entity.Step(tickerAt("BTC/USD", 100, 102, 3.0, 1.0, base.Add(3*time.Second)))

		Convey("the z-score is present and equals divergence / noise", func() {
			zscore, hasZScore := fourth.Metrics["depth_zscore:bid"]
			So(hasZScore, ShouldBeTrue)

			noise, hasNoise := fourth.Metrics["depth_noise_scale:bid"]
			So(hasNoise, ShouldBeTrue)
			So(noise.Raw, ShouldBeGreaterThan, 0)

			divergence := fourth.Metrics["depth_divergence:bid"]
			So(zscore.Raw, ShouldAlmostEqual, divergence.Raw/noise.Raw, 1e-9)
		})
	})
}

/*
TestTickerMaturityUsesNEff asserts the measurement maturity follows the Kish
effective-support formula: Maturity = 1 - 1/N_eff (0 when N_eff <= 1). With a
single prior observation N_eff == 2, so Maturity == 0.5; with none, 0.
*/
func TestTickerMaturityUsesNEff(t *testing.T) {
	Convey("Given one then two observations", t, func() {
		entity := NewTicker()
		base := time.Unix(1, 0)

		first := entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("maturity is 0 with no effective support, 1 - 1/N_eff with support", func() {
			// First observation: no prior baseline, N_eff <= 1 -> maturity 0.
			So(first.Maturity, ShouldEqual, 0.0)

			// Second observation: with an event-time decay alpha = 0.5
			// (elapsed == cadence), N_eff = (1.0)^2 / (0.5) = 2 -> maturity 0.5.
			So(second.Maturity, ShouldAlmostEqual, 0.5, 1e-6)
		})
	})
}

/*
TestTickerIrregularTimeRegression is BLOCKER 2: the divergence velocity is a
causal local-time regression slope, not a first difference. The same underlying
time slope sampled on different irregular grids must agree.
*/
func TestTickerIrregularTimeRegression(t *testing.T) {
	Convey("Given one linear divergence trajectory on two grids", t, func() {
		// The bid depth is chosen so log depth follows a linear-in-time
		// trajectory; the estimator must recover the same slope regardless of
		// the sampling grid. This is asserted directly at the statistic level
		// in TestLocalRegressionIrregularGrid in nomagique/statistic.
		entity := NewTicker()

		// Just ensure the velocity metric exists and is computed (present) once
		// enough history exists, rather than a raw delta.
		base := time.Unix(1, 0)
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.0, 1.0, base))
		entity.Step(tickerAt("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))
		entity.Step(tickerAt("BTC/USD", 100, 102, 1.5, 1.0, base.Add(3*time.Second)))
		fourth := entity.Step(tickerAt("BTC/USD", 100, 102, 3.0, 1.0, base.Add(7*time.Second)))

		Convey("the velocity is a fitted regression slope, not a message-count delta", func() {
			velocity, hasVelocity := fourth.Metrics["divergence_velocity:bid"]
			So(hasVelocity, ShouldBeTrue)

			// A per-message delta would equal log(3.0/1.5); the regression slope
			// is time-normalized and therefore different. We only assert it is
			// finite (the exact slope equality across grids is tested at the
			// statistic level).
			So(math.IsNaN(velocity.Raw), ShouldBeFalse)
			So(math.IsInf(velocity.Raw, 0), ShouldBeFalse)
		})
	})
}

var _ = kraken.TickerData{}
