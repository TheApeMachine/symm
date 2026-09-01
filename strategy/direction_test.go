package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDirectionalPredictorAdvance(t *testing.T) {
	Convey("Given a precursor whose sign repeatedly determines both next movement outcomes", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:       1,
			forgettingFactor:      1,
			calibrationConfidence: 0.95,
		})
		So(err, ShouldBeNil)

		reference := 100.0
		previousPrecursor := 0.0
		var forecast *directionalForecast

		for step := 0; step < 255; step++ {
			precursor := -1.0

			if step%2 == 0 {
				precursor = 1
			}

			reference += previousPrecursor
			breakEven := reference
			measurement := data.NewMeasurement[float64](
				"precursor", "TEST/USD", "test", time.Unix(int64(step), 0), time.Time{},
			)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "signed_precursor", Raw: precursor})
			err = predictor.observeMeasurement(measurement)
			So(err, ShouldBeNil)

			forecast, err = predictor.advance(
				"TEST/USD", time.Unix(int64(step), 0), reference, reference, &breakEven,
			)
			So(err, ShouldBeNil)
			previousPrecursor = precursor
		}

		Convey("the model becomes ready from measured out-of-sample skill", func() {
			So(forecast.directionReady, ShouldBeTrue)
			So(forecast.profitabilityReady, ShouldBeTrue)
			So(forecast.directionSkillLowerBound, ShouldBeGreaterThan, 0.0)
			So(forecast.profitSkillLowerBound, ShouldBeGreaterThan, 0.0)
		})

		Convey("it classifies direction and profitability without estimating an amount", func() {
			So(forecast.probabilityUp, ShouldBeGreaterThan, 0.5)
			So(forecast.probabilityProfitable, ShouldBeGreaterThan, 0.5)
			So(forecast.directionFeatures, ShouldEqual, 1)
			So(forecast.profitFeatures, ShouldEqual, 1)
		})
	})
}

func BenchmarkDirectionalPredictorAdvance(b *testing.B) {
	predictor, err := newDirectionalPredictor(directionalConfig{
		initialVariance:       1,
		forgettingFactor:      1,
		calibrationConfidence: 0.95,
	})

	if err != nil {
		b.Fatal(err)
	}

	measurement := data.NewMeasurement[float64](
		"precursor", "TEST/USD", "test", time.Unix(1, 0), time.Time{},
	)
	measurement.Maturity = 1
	measurement.PutMetric(data.Metric[float64]{Label: "signed_precursor", Raw: 1})

	if err := predictor.observeMeasurement(measurement); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for step := 0; b.Loop(); step++ {
		breakEven := 100.0

		if _, err := predictor.advance(
			"TEST/USD", time.Unix(int64(step), 0),
			100+float64(step%2), 100+float64(step%2), &breakEven,
		); err != nil {
			b.Fatal(err)
		}
	}
}
