package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestDirectionalPredictorAdvance(t *testing.T) {
	Convey("Given context that repeatedly precedes executable upward movement", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)

		price := 100.0
		previousContext := 0.0
		var forecast *directionalForecast

		for step := 0; step < 256; step++ {
			at := time.Unix(int64(step), 0)
			price *= math.Exp(previousContext / 100)
			context := -1.0

			if step%2 != 0 {
				context = 1
			}

			measurement := data.NewMeasurement[float64](
				"activity", "TEST/USD", "pumpdump", at, time.Time{},
			)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{
				Label: "volume_bar_duration", Raw: 1,
			})
			measurement.PutMetric(data.Metric[float64]{
				Label: "midpoint_return_rate", Raw: context,
			})
			So(predictor.observeMeasurement(measurement), ShouldBeNil)
			So(predictor.observeOpportunity(&types.OpportunityCandidate{
				Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
				Phase: types.PhaseArmed, Direction: types.DirectionLong,
				Updated: at, Maturity: 1,
			}), ShouldBeNil)

			breakEven := price * math.Exp(1.0/1000)
			forecast, err = predictor.advance("TEST/USD", at, price, &breakEven)
			So(err, ShouldBeNil)
			previousContext = context
		}

		Convey("the model exposes one calibrated return distribution", func() {
			So(forecast.ready, ShouldBeTrue)
			So(forecast.output.Ready, ShouldBeTrue)
			So(forecast.calibration, ShouldBeGreaterThan, 0)
			So(forecast.horizon, ShouldEqual, time.Second)
			So(forecast.horizonSteps, ShouldEqual, 1)
		})

		Convey("upward and profitable likelihoods are queries of that distribution", func() {
			So(forecast.expectedLogReturn, ShouldBeGreaterThan, forecast.breakEvenLogReturn)
			So(forecast.probabilityUp, ShouldBeGreaterThan, 0.5)
			So(forecast.probabilityProfitable, ShouldBeGreaterThan, 0.5)
			So(forecast.directionalFeatures, ShouldBeGreaterThan, 0)
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

		breakEven := 100.1

		if _, err := predictor.advance(
			"TEST/USD", at, 100+float64(step%2), &breakEven,
		); err != nil {
			b.Fatal(err)
		}
	}
}
