package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

func TestDirectionalPredictorAdvance(t *testing.T) {
	Convey("Given context that repeatedly precedes executable upward movement", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)

		tape := markettest.NewOpportunityTape(
			"TEST/USD", time.Unix(1_700_000_000, 0), 64,
		)
		So(predictor.observeResonance(&types.ResonanceArtifact{
			Symbol: tape.Symbol, Calibrated: true,
			SupportedHorizon: tape.HorizonSteps, At: tape.Steps[0].EventTime,
		}), ShouldBeNil)
		executionFactor := math.Sqrt(
			tape.Steps[tape.HorizonSteps].ExecutableBid /
				tape.Steps[0].ExecutableBid,
		)

		var forecast *directionalForecast

		for _, step := range tape.Steps {
			measurement := data.NewMeasurement[float64](
				"activity", tape.Symbol, "pumpdump", step.EventTime, time.Time{},
			)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{
				Label: "midpoint_return_rate", Raw: step.Context,
			})
			So(predictor.observeMeasurement(measurement), ShouldBeNil)
			So(predictor.observeOpportunity(&types.OpportunityCandidate{
				Symbol: tape.Symbol, Archetype: types.ArchetypeVerticalIgnition,
				Phase: types.PhaseArmed, Direction: types.DirectionLong,
				Updated: step.EventTime, Maturity: 1,
			}), ShouldBeNil)

			breakEven := step.ExecutableBid
			forecast, err = predictor.advance(
				tape.Symbol, step.EventTime, step.ExecutableBid,
			)
			So(err, ShouldBeNil)
			breakEven *= executionFactor
			So(forecast.observeExecutionBoundary(
				step.ExecutableBid, breakEven,
			), ShouldBeNil)
		}

		Convey("the model exposes one calibrated return distribution", func() {
			So(forecast.ready, ShouldBeTrue)
			So(forecast.output.Ready, ShouldBeTrue)
			So(forecast.calibration, ShouldBeGreaterThan, 0)
			So(forecast.horizonSteps, ShouldEqual, tape.HorizonSteps)
		})

		Convey("upward and profitable likelihoods are queries of that distribution", func() {
			So(forecast.expectedLogReturn, ShouldBeGreaterThan, forecast.breakEvenLogReturn)
			So(forecast.probabilityUp, ShouldBeGreaterThan, 0.5)
			So(forecast.probabilityProfitable, ShouldBeGreaterThan, 0.5)
			So(forecast.probabilityProfitable, ShouldBeLessThan, forecast.probabilityUp)
			So(forecast.directionalFeatures, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a return posterior with only one resolved outcome", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		So(predictor.observeResonance(&types.ResonanceArtifact{
			Symbol: "TEST/USD", Calibrated: true, SupportedHorizon: 1,
			At: time.Unix(1, 0),
		}), ShouldBeNil)
		So(predictor.observeOpportunity(&types.OpportunityCandidate{
			Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
			Phase: types.PhaseArmed, Direction: types.DirectionLong,
			Updated: time.Unix(1, 0), Maturity: 1,
		}), ShouldBeNil)

		_, err = predictor.advance("TEST/USD", time.Unix(1, 0), 100)
		So(err, ShouldBeNil)
		forecast, err := predictor.advance(
			"TEST/USD", time.Unix(2, 0), 102,
		)
		So(err, ShouldBeNil)
		So(forecast.observeExecutionBoundary(102, 103), ShouldBeNil)

		Convey("its undefined mean cannot be presented as an expected return", func() {
			So(forecast.output.Ready, ShouldBeTrue)
			So(forecast.output.DegreesOfFreedom, ShouldEqual, 1.0)
			So(forecast.ready, ShouldBeFalse)
			So(forecast.expectedLogReturn, ShouldEqual, 0.0)
			So(forecast.status, ShouldEqual, "return-distribution-mean-undefined")
		})

		Convey("the mean becomes usable only when it mathematically exists", func() {
			forecast, err = predictor.advance(
				"TEST/USD", time.Unix(3, 0), 104,
			)
			So(err, ShouldBeNil)
			So(forecast.observeExecutionBoundary(104, 105), ShouldBeNil)
			So(forecast.output.DegreesOfFreedom, ShouldBeGreaterThan, 1.0)
			So(forecast.ready, ShouldBeTrue)
			So(forecast.expectedLogReturn, ShouldEqual, forecast.output.Value)
		})
	})

	Convey("Given facts committed on both sides of a ticker's event timestamp", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		So(predictor.observeResonance(&types.ResonanceArtifact{
			Symbol: "TEST/USD", At: time.Unix(2, 0),
		}), ShouldBeNil)
		So(predictor.observeOpportunity(&types.OpportunityCandidate{
			Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
			Phase: types.PhaseArmed, Direction: types.DirectionLong,
			Updated: time.Unix(2, 0), Maturity: 1,
		}), ShouldBeNil)

		older := data.NewMeasurement[float64](
			"review", "TEST/USD", "undeclared", time.Unix(0, 0), time.Time{},
		)
		older.Maturity = 1
		older.PutMetric(data.Metric[float64]{Label: "fact", Raw: 1})
		So(predictor.observeMeasurement(older), ShouldBeNil)

		forecast, err := predictor.advance("TEST/USD", time.Unix(1, 0), 100)

		Convey("all causally known facts remain available at the decision cut", func() {
			So(err, ShouldBeNil)
			So(forecast, ShouldNotBeNil)
			So(forecast.reviewFeatures, ShouldEqual, 1)
		})
	})
}

func TestDirectionalPredictorObserveMeasurement(t *testing.T) {
	Convey("Given a calibrated ticker-step horizon", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		So(predictor.observeResonance(&types.ResonanceArtifact{
			Symbol: "TEST/USD", Calibrated: true, SupportedHorizon: 7,
			At: time.Unix(1, 0),
		}), ShouldBeNil)

		measurement := data.NewMeasurement[float64](
			"activity", "TEST/USD", "pumpdump", time.Unix(1, 0), time.Time{},
		)
		measurement.Maturity = 1
		measurement.PutMetric(data.Metric[float64]{
			Label: "volume_bar_duration", Raw: 0.000001,
		})
		So(predictor.observeMeasurement(measurement), ShouldBeNil)

		state, err := predictor.state("TEST/USD")
		So(err, ShouldBeNil)

		Convey("volume-bar cadence remains a feature and cannot relabel payoff time", func() {
			So(state.horizonSteps, ShouldEqual, 7)
			So(state.features[featureKey{
				family: "measurement", source: "pumpdump", metric: "volume_bar_duration",
			}], ShouldNotBeNil)
		})
	})
}

func TestDirectionalForecastObserveExecutionBoundary(t *testing.T) {
	Convey("Given an execution boundary", t, func() {
		forecast := &directionalForecast{}

		Convey("non-positive economics fail instead of opening a zero hurdle", func() {
			So(forecast.observeExecutionBoundary(0, 1), ShouldNotBeNil)
			So(forecast.observeExecutionBoundary(1, 0), ShouldNotBeNil)
			So(forecast.ready, ShouldBeFalse)
		})

		Convey("a positive boundary is retained even before calibration", func() {
			So(forecast.observeExecutionBoundary(100, 101), ShouldBeNil)
			So(forecast.breakEvenLogReturn, ShouldAlmostEqual, math.Log(101.0/100.0))
			So(forecast.ready, ShouldBeFalse)
		})
	})
}

func TestSemanticFeatureUse(t *testing.T) {
	Convey("Given metric-map roles used by the advisee", t, func() {
		Convey("market context may inform the conditional return distribution", func() {
			So(semanticFeatureUse("cvd", "signed_net_fraction"), ShouldEqual, featureContext)
		})

		Convey("estimator quality cannot impersonate direction", func() {
			So(semanticFeatureUse("correlation", "correlation_p_value"), ShouldEqual, featureEstimability)
		})

		Convey("execution facts remain execution context", func() {
			So(semanticFeatureUse("liquidity", "relative_spread"), ShouldEqual, featureExecution)
		})

		Convey("undeclared facts are held for semantic review", func() {
			So(semanticFeatureUse("undeclared", "fact"), ShouldEqual, featureReview)
		})
	})
}

func BenchmarkDirectionalPredictorAdvance(b *testing.B) {
	predictor, err := newDirectionalPredictor(directionalConfig{
		initialVariance:  1,
		forgettingFactor: 1,
	})

	if err != nil {
		b.Fatal(err)
	}

	if err := predictor.observeResonance(&types.ResonanceArtifact{
		Symbol: "TEST/USD", Calibrated: true, SupportedHorizon: 3,
		At: time.Unix(1, 0),
	}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for step := 0; b.Loop(); step++ {
		at := time.Unix(int64(step), 0)
		measurement := data.NewMeasurement[float64](
			"activity", "TEST/USD", "pumpdump", at, time.Time{},
		)
		measurement.Maturity = 1
		measurement.PutMetric(data.Metric[float64]{Label: "volume_bar_duration", Raw: 1})
		measurement.PutMetric(data.Metric[float64]{Label: "midpoint_return_rate", Raw: float64(step % 2)})

		if err := predictor.observeMeasurement(measurement); err != nil {
			b.Fatal(err)
		}

		if err := predictor.observeOpportunity(&types.OpportunityCandidate{
			Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
			Phase: types.PhaseArmed, Direction: types.DirectionLong,
			Updated: at, Maturity: 1,
		}); err != nil {
			b.Fatal(err)
		}

		executableBid := 100 + float64(step%2)
		forecast, err := predictor.advance("TEST/USD", at, executableBid)

		if err != nil {
			b.Fatal(err)
		}

		if err := forecast.observeExecutionBoundary(executableBid, 101.1); err != nil {
			b.Fatal(err)
		}
	}
}
