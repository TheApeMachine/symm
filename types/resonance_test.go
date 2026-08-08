package types

import (
	"encoding/json"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewResonanceForecast(t *testing.T) {
	Convey("Given materially different supported forecast horizons", t, func() {
		oneStepCurve := []float64{0.01}
		longCurve := []float64{0.01, 0.02, -0.005}
		longRetention := []float64{1, 0.5, 0.25}

		oneStep, err := NewResonanceForecast(oneStepCurve, []float64{1}, 1, 0.25)
		So(err, ShouldBeNil)

		long, err := NewResonanceForecast(longCurve, longRetention, len(longCurve), 0.75)
		So(err, ShouldBeNil)

		Convey("It should evaluate every confidence-supported step", func() {
			expectedLongReturn := math.Expm1(
				longCurve[0] + longCurve[1]*longRetention[1] -
					math.Abs(longCurve[2])*longRetention[2],
			)

			So(oneStep.ExpectedReturn, ShouldAlmostEqual, math.Expm1(oneStepCurve[0]), 1e-12)
			So(long.ExpectedReturn, ShouldAlmostEqual, expectedLongReturn, 1e-12)
			So(long.ExpectedBasisPoints, ShouldAlmostEqual,
				expectedLongReturn*basisPointsPerUnit, 1e-12)
			So(long.ExpectedReturn, ShouldNotEqual, oneStep.ExpectedReturn)
			So(long.SupportedHorizon, ShouldEqual, len(longCurve))
			So(long.Confidence, ShouldEqual, 0.75)
		})

		Convey("It should own its curve independently from the caller", func() {
			longCurve[0] = 1
			longRetention[1] = 1

			So(long.Curve[0], ShouldEqual, 0.01)
			So(long.Retention[1], ShouldEqual, 0.5)
			So(long.Validate(), ShouldBeNil)
		})
	})

	Convey("Given a valid forecast of no movement", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0, 0}, []float64{1, 0.75}, 2, 0.5,
		)

		Convey("It should preserve a genuine numerical zero", func() {
			So(err, ShouldBeNil)
			So(forecast.ExpectedReturn, ShouldEqual, 0.0)
			step, present := forecast.Step(1)
			So(present, ShouldBeTrue)
			So(step, ShouldEqual, 0.0)
		})
	})

	Convey("Given incomplete or malformed forecast evidence", t, func() {
		cases := []struct {
			curve      []float64
			retention  []float64
			horizon    int
			confidence float64
		}{
			{[]float64{0.01, 0.02}, []float64{1}, 2, 0.5},
			{[]float64{0.01}, []float64{0}, 1, 0.5},
			{[]float64{math.NaN()}, []float64{1}, 1, 0.5},
			{[]float64{0.01}, []float64{1}, 1, math.NaN()},
		}

		Convey("It should refuse to fabricate a supported return", func() {
			for _, testCase := range cases {
				forecast, err := NewResonanceForecast(
					testCase.curve,
					testCase.retention,
					testCase.horizon,
					testCase.confidence,
				)

				So(err, ShouldNotBeNil)
				So(forecast, ShouldBeNil)
			}
		})
	})
}

func TestResonanceForecastSetPredictiveDistribution(t *testing.T) {
	Convey("Given an identified Student-t predictive distribution", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0.001}, []float64{1}, 1, 0.8,
		)
		So(err, ShouldBeNil)

		err = forecast.SetPredictiveDistribution(0.0004, 12, true)

		Convey("It should retain the distribution and its basis-point scale", func() {
			So(err, ShouldBeNil)
			So(forecast.ConfidenceReady, ShouldBeTrue)
			So(forecast.PredictiveScale, ShouldEqual, 0.0004)
			So(forecast.PredictiveScaleBasisPoints, ShouldEqual, 4.0)
			So(forecast.DegreesOfFreedom, ShouldEqual, 12)
			So(forecast.Validate(), ShouldBeNil)
		})
	})

	Convey("Given uncertainty that is not identified", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0}, []float64{1}, 1, 0.5,
		)
		So(err, ShouldBeNil)

		err = forecast.SetPredictiveDistribution(1, 0, false)

		Convey("It should reject an invented unavailable scale", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a forecast whose former distribution is no longer available", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0}, []float64{1}, 1, 0.5,
		)
		So(err, ShouldBeNil)
		So(forecast.SetPredictiveDistribution(0.0004, 12, true), ShouldBeNil)

		err = forecast.SetPredictiveDistribution(0, 0, false)

		Convey("It should remove every stale uncertainty field", func() {
			So(err, ShouldBeNil)
			So(forecast.ConfidenceReady, ShouldBeFalse)
			So(forecast.PredictiveScale, ShouldEqual, 0.0)
			So(forecast.PredictiveScaleBasisPoints, ShouldEqual, 0.0)
			So(forecast.DegreesOfFreedom, ShouldEqual, 0.0)
			So(forecast.Validate(), ShouldBeNil)

			payload, err := json.Marshal(forecast)
			So(err, ShouldBeNil)
			So(string(payload), ShouldNotContainSubstring, "predictiveScale")
			So(string(payload), ShouldNotContainSubstring, "degreesOfFreedom")
		})
	})
}

func TestResonanceForecastValidate(t *testing.T) {
	Convey("Given a published forecast whose derived return was altered", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0.01, 0.02}, []float64{1, 0.5}, 2, 0.75,
		)
		So(err, ShouldBeNil)

		forecast.ExpectedReturn = math.Expm1(forecast.Curve[0])

		Convey("It should expose the semantic mismatch", func() {
			So(forecast.Validate(), ShouldNotBeNil)
		})
	})
}

func TestResonanceForecastStep(t *testing.T) {
	Convey("Given a retained multi-step forecast", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0.01, 0.02}, []float64{1, 0.5}, 2, 0.75,
		)
		So(err, ShouldBeNil)

		Convey("It should expose the supported value at each step", func() {
			first, firstPresent := forecast.Step(0)
			second, secondPresent := forecast.Step(1)

			So(firstPresent, ShouldBeTrue)
			So(first, ShouldEqual, 0.01)
			So(secondPresent, ShouldBeTrue)
			So(second, ShouldEqual, 0.01)
		})

		Convey("It should distinguish an absent step from numerical zero", func() {
			_, present := forecast.Step(forecast.SupportedHorizon)
			So(present, ShouldBeFalse)
		})
	})
}

func TestResonanceForecastWorstIntermediateDrawdown(t *testing.T) {
	Convey("Given a retained forecast path with a temporary intermediate dip", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{-0.01, -0.02, 0.06},
			[]float64{1, 0.5, 0.5},
			3,
			0.9,
		)
		So(err, ShouldBeNil)

		Convey("It should report the deepest cumulative retained loss", func() {
			drawdown, err := forecast.WorstIntermediateDrawdown()

			So(err, ShouldBeNil)
			So(drawdown, ShouldAlmostEqual, -math.Expm1(-0.02), 1e-12)
		})
	})

	Convey("Given a path that remains above its starting price", t, func() {
		forecast, err := NewResonanceForecast(
			[]float64{0.02, -0.01},
			[]float64{1, 1},
			2,
			0.9,
		)
		So(err, ShouldBeNil)

		Convey("It should report no adverse excursion from entry", func() {
			drawdown, err := forecast.WorstIntermediateDrawdown()

			So(err, ShouldBeNil)
			So(drawdown, ShouldEqual, 0.0)
		})
	})
}

func BenchmarkNewResonanceForecast(b *testing.B) {
	curve := make([]float64, 20)
	retention := make([]float64, len(curve))

	for index := range curve {
		curve[index] = float64(index+1) / 10000
		retention[index] = 1 - float64(index)/float64(len(curve)*2)
	}

	b.ResetTimer()

	for b.Loop() {
		_, err := NewResonanceForecast(curve, retention, len(curve), 0.75)

		if err != nil {
			b.Fatal(err)
		}
	}
}
