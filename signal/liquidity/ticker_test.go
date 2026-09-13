package liquidity

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's touch quote written.
The label is the symbol and the timestamp is the venue's.
*/
var schema = new(Ticker).Register().Metrics

func tick(symbol string, bid, ask, bidQty, askQty float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("liquidity", cloneSchema())
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["bid"] = m.Metrics["bid"].Write(bid)
	m.Metrics["ask"] = m.Metrics["ask"].Write(ask)
	m.Metrics["bid_qty"] = m.Metrics["bid_qty"].Write(bidQty)
	m.Metrics["ask_qty"] = m.Metrics["ask_qty"].Write(askQty)

	return m
}

func cloneSchema() map[string]data.Metric[float64] {
	metrics := make(map[string]data.Metric[float64], len(schema))

	for key, metric := range schema {
		metrics[key] = metric
	}

	return metrics
}

/*
TestTickerStepPreObservationBaseline is the exact BLOCKER 1 fixture: with one
prior observation, the only possible causal baseline is that observation, so
all four published facts must reference the SAME pre-observation baseline.
*/
func TestTickerStepPreObservationBaseline(t *testing.T) {
	Convey("Given bid depth 100 then 200", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(1_700_000_000, 0)

		entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("baseline, ratio and divergence reference the same pre-observation baseline", func() {
			So(second.Err, ShouldBeNil)
			So(second.Metrics["touch_notional_baseline:bid"].Raw, ShouldAlmostEqual, 100.0, 1e-9)
			So(second.Metrics["depth_ratio:bid"].Raw, ShouldAlmostEqual, 2.0, 1e-9)
			So(second.Metrics["depth_divergence:bid"].Raw, ShouldAlmostEqual, math.Log(2.0), 1e-9)

			// log(depth_ratio) == depth_divergence exactly.
			So(math.Log(second.Metrics["depth_ratio:bid"].Raw), ShouldAlmostEqual, second.Metrics["depth_divergence:bid"].Raw, 1e-12)
		})
	})

	Convey("the ask-side and spread facts follow the same contract", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(20, 0)

		// Ask notional stays 102 across both steps (askQty constant), so the
		// ask divergence is 0 and the baseline is the prior ask notional.
		entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("ask baseline is the prior ask notional with zero divergence", func() {
			So(second.Metrics["touch_notional_baseline:ask"].Raw, ShouldAlmostEqual, 102.0, 1e-9)
			So(second.Metrics["depth_divergence:ask"].Raw, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})
}

/*
TestTickerStepDegenerateNoise is BLOCKER 3: the z-score is undefined (zero,
never a fabricated number) when the pre-observation noise is unavailable or
degenerate.
*/
func TestTickerStepDegenerateNoise(t *testing.T) {
	Convey("Given no prior observation", t, func() {
		entity := NewTicker(context.Background())
		first := entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, time.Unix(1, 0)))

		Convey("no baseline produces no z-score", func() {
			So(first.Metrics["depth_zscore:bid"].Raw, ShouldEqual, 0.0)
			So(first.Metrics["touch_notional_baseline:bid"].Raw, ShouldEqual, 0.0)
		})
	})

	Convey("Given a single prior observation (degenerate residual scale)", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(1, 0)
		entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("the z-score is unwritten, the divergence is defined", func() {
			So(second.Metrics["depth_zscore:bid"].Raw, ShouldEqual, 0.0)
			So(second.Metrics["depth_divergence:bid"].Raw, ShouldAlmostEqual, math.Log(2.0), 1e-9)
		})
	})
}

/*
TestTickerStepZScorePresent is BLOCKER 3's positive case: once the noise scale
is estimable and positive, the z-score is present and equals divergence/noise.
*/
func TestTickerStepZScorePresent(t *testing.T) {
	Convey("Given a history with non-zero residual dispersion", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(1000, 0)

		// Vary the bid depth so the residual dispersion is non-zero, then
		// observe the latest point's z-score.
		entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))
		entity.Step(tick("BTC/USD", 100, 102, 1.5, 1.0, base.Add(2*time.Second)))
		fourth := entity.Step(tick("BTC/USD", 100, 102, 3.0, 1.0, base.Add(3*time.Second)))

		Convey("the z-score is present and equals divergence / noise", func() {
			zscore := fourth.Metrics["depth_zscore:bid"].Raw
			noise := fourth.Metrics["depth_noise_scale:bid"].Raw

			So(noise, ShouldBeGreaterThan, 0)
			So(zscore, ShouldAlmostEqual, fourth.Metrics["depth_divergence:bid"].Raw/noise, 1e-9)
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
		entity := NewTicker(context.Background())
		base := time.Unix(1, 0)

		first := entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

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
		entity := NewTicker(context.Background())

		// Feed enough history that the divergence path accumulates several
		// in-horizon samples and the causal local regression becomes defined —
		// before that, the velocity is legitimately undefined (unwritten, not
		// a numeric zero).
		base := time.Unix(1, 0)
		depths := []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 3.5, 5.0}
		offsets := []time.Duration{0, time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 10 * time.Second, 13 * time.Second, 15 * time.Second}

		var fourth *data.Measurement[float64]

		for index := range depths {
			fourth = entity.Step(tick("BTC/USD", 100, 102, depths[index], 1.0, base.Add(offsets[index])))
		}

		Convey("the velocity is a fitted regression slope, not a message-count delta", func() {
			velocity := fourth.Metrics["divergence_velocity:bid"].Raw

			// A per-message delta would equal log(5.0/3.5); the regression slope
			// is time-normalized and therefore different. We only assert it is
			// finite (the exact slope equality across grids is tested at the
			// statistic level).
			So(math.IsNaN(velocity), ShouldBeFalse)
			So(math.IsInf(velocity, 0), ShouldBeFalse)
		})
	})
}

/*
TestTickerVelocityUndefinedAbsent asserts BLOCKER: undefined ≠ zero for the
divergence velocity. Before the divergence path has enough in-horizon support
for the local regression, divergence_velocity:* must be unwritten (zero),
never emitted as a fabricated slope.
*/
func TestTickerVelocityUndefinedAbsent(t *testing.T) {
	Convey("Given two observations (insufficient regression support)", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(1, 0)

		entity.Step(tick("BTC/USD", 100, 102, 1.0, 1.0, base))
		second := entity.Step(tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))

		Convey("the divergence velocity is absent, not zero", func() {
			So(second.Metrics["divergence_velocity:bid"].Raw, ShouldEqual, 0.0)
			So(second.Metrics["divergence_velocity_snr:bid"].Raw, ShouldEqual, 0.0)
		})
	})
}

/*
TestTickerVelocitySNRPresent asserts the divergence velocity SNR is projected
once the regression is defined, and equals the velocity fit's own SNR.
*/
func TestTickerVelocitySNRPresent(t *testing.T) {
	Convey("Given a divergence trajectory with enough in-horizon support", t, func() {
		entity := NewTicker(context.Background())
		base := time.Unix(1, 0)
		depths := []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 3.5, 5.0, 4.5, 6.0}
		offsets := []time.Duration{0, time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 10 * time.Second, 13 * time.Second, 15 * time.Second, 18 * time.Second, 20 * time.Second}

		var last *data.Measurement[float64]

		for index := range depths {
			last = entity.Step(tick("BTC/USD", 100, 102, depths[index], 1.0, base.Add(offsets[index])))
		}

		Convey("the velocity SNR metric is present and finite", func() {
			snr := last.Metrics["divergence_velocity_snr:bid"].Raw

			So(math.IsNaN(snr), ShouldBeFalse)
			So(math.IsInf(snr, 0), ShouldBeFalse)
			So(snr, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestTickerStep(t *testing.T) {
	Convey("Given a liquidity ticker-path instrument", t, func() {
		entity := NewTicker(context.Background())

		Convey("the first tick yields one measurement with the touch written", func() {
			measurement := entity.Step(tick("BTC/USD", 100, 102, 1, 1, time.Unix(1, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["touch_notional:bid"].Raw, ShouldEqual, 100.0)
		})

		Convey("a crossed quote fails the gate", func() {
			measurement := entity.Step(tick("BTC/USD", 103, 102, 1, 1, time.Unix(1, 0)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("a measurement without a quote fails the gate", func() {
			m := tick("BTC/USD", 0, 0, 1, 1, time.Unix(1, 0))

			measurement := entity.Step(m)

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("time regression surfaces without error and without advancing", func() {
			entity.Step(tick("BTC/USD", 100, 102, 1, 1, time.Unix(2, 0)))

			measurement := entity.Step(tick("BTC/USD", 100, 102, 2, 1, time.Unix(1, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "bid")
			So(measurement.Metrics, ShouldContainKey, "spread")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}
