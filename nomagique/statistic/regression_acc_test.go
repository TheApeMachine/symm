package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewRegressionAccumulator(t *testing.T) {
	Convey("Given a stream of y = 1 + 2x rows with an intercept column", t, func() {
		rows := make([]RegressionRow, 0, 20)

		for index := 0; index < 20; index++ {
			x := float64(index) / 5
			rows = append(rows, RegressionRow{Predictors: []float64{1, x}, Target: 1 + 2*x})
		}

		readings := collectReadings[RegressionRow, RegressionReading](t, NewRegressionAccumulator(2), rows)

		Convey("every row yields exactly one reading", func() {
			So(len(readings), ShouldEqual, 20)
		})

		Convey("warm-up rows are undefined and not predicted", func() {
			So(readings[0].PredictionDefined, ShouldBeFalse)
			So(readings[1].PredictionDefined, ShouldBeFalse)
			So(readings[1].Fit.Defined, ShouldBeFalse)
		})

		Convey("prequential prediction starts one row after the RLS seed", func() {
			So(readings[2].PredictionDefined, ShouldBeFalse)
			So(readings[3].PredictionDefined, ShouldBeTrue)
			So(readings[3].Prediction, ShouldAlmostEqual, 1+2*(3.0/5.0), 1e-9)
		})

		Convey("the final fit recovers the true line exactly", func() {
			final := readings[len(readings)-1].Fit
			So(final.Defined, ShouldBeTrue)
			So(final.Observations, ShouldEqual, 20)
			So(final.Parameters, ShouldEqual, 2)
			So(final.Coefficients[0], ShouldAlmostEqual, 1, 1e-9)
			So(final.Coefficients[1], ShouldAlmostEqual, 2, 1e-9)
			So(final.ResidualSSE, ShouldAlmostEqual, 0, 1e-9)
			So(len(final.CoefficientVariance), ShouldEqual, 2)
		})
	})
}

func TestNewRegressionAccumulatorDomain(t *testing.T) {
	Convey("Given a non-positive parameter count", t, func() {
		operation := NewRegressionAccumulator(0)
		yielded := 0

		for range operation.Next(transport.NewValues(RegressionRow{Predictors: []float64{1}, Target: 1}).Next(nil)) {
			yielded++
		}

		Convey("the run is empty and the domain violation is recorded", func() {
			So(yielded, ShouldEqual, 0)
			So(operation.Error(), ShouldNotBeNil)
		})
	})
}

func TestNewRegressionAccumulatorShape(t *testing.T) {
	Convey("Given a row whose length differs from the parameter count", t, func() {
		operation := NewRegressionAccumulator(2)
		yielded := 0

		for range operation.Next(transport.NewValues(RegressionRow{Predictors: []float64{1}, Target: 1}).Next(nil)) {
			yielded++
		}

		Convey("the run stops and records the shape violation", func() {
			So(yielded, ShouldEqual, 0)
			So(operation.Error(), ShouldNotBeNil)
		})
	})
}

func TestNewRegressionAccumulatorRankDeficient(t *testing.T) {
	Convey("Given duplicate design columns", t, func() {
		rows := make([]RegressionRow, 0, 10)

		for index := 0; index < 10; index++ {
			x := float64(index)
			rows = append(rows, RegressionRow{Predictors: []float64{1, x, x}, Target: x})
		}

		readings := collectReadings[RegressionRow, RegressionReading](t, NewRegressionAccumulator(3), rows)

		Convey("the fit stays undefined rather than regularized", func() {
			So(math.IsNaN(readings[len(readings)-1].Fit.ResidualVariance), ShouldBeTrue)
			So(readings[len(readings)-1].Fit.Defined, ShouldBeFalse)
		})
	})
}
