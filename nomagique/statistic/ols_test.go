package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
evaluateOLS fits one request through the OLS Primitive.
*/
func evaluateOLS(t *testing.T, request OLSRequest) OLSFit {
	t.Helper()

	fitEval := transport.NewEvaluate(NewFitOLS())
	var fit OLSFit

	for out := range fitEval.Next(transport.NewValues(request).Next(nil)) {
		fit = *(*OLSFit)(out)
	}

	err := fitEval.Error()

	if err != nil {
		t.Fatalf("OLS evaluation: %v", err)
	}

	return fit
}

/*
evaluateSNR scores one pair through the coefficient SNR Primitive.
*/
func evaluateSNR(t *testing.T, pair CoefficientSNRPair) float64 {
	t.Helper()

	snrEval := transport.NewEvaluate(NewCoefficientSNR())
	var snr float64

	for out := range snrEval.Next(transport.NewValues(pair).Next(nil)) {
		snr = *(*float64)(out)
	}

	err := snrEval.Error()

	if err != nil {
		t.Fatalf("coefficient SNR evaluation: %v", err)
	}

	return snr
}

func TestFitOLSNext(t *testing.T) {
	Convey("Given a linear system y = 1 + 2x", t, func() {
		design := make([]float64, 0, 100)
		targets := make([]float64, 0, 50)

		for index := 0; index < 50; index++ {
			x := float64(index) / 10
			design = append(design, 1, x)
			targets = append(targets, 1+2*x+0.001*float64(index%3-1))
		}

		fit := evaluateOLS(t, OLSRequest{X: design, Y: targets, P: 2})

		Convey("the fit is defined with the true coefficients", func() {
			So(fit.Defined, ShouldBeTrue)
			So(fit.Rank, ShouldEqual, 2)
			So(fit.Coefficients[1], ShouldAlmostEqual, 2, 0.01)
			So(fit.Coefficients[0], ShouldAlmostEqual, 1, 0.05)
		})

		Convey("duplicate columns are rank-deficient, not silently regularized", func() {
			duplicated := make([]float64, 0, 150)
			for index := 0; index < 50; index++ {
				x := float64(index) / 10
				duplicated = append(duplicated, 1, x, x)
			}

			deficient := evaluateOLS(t, OLSRequest{X: duplicated, Y: targets, P: 3})
			So(deficient.Defined, ShouldBeFalse)
			So(deficient.Rank, ShouldBeLessThan, 3)
		})

		Convey("insufficient rows are undefined", func() {
			small := evaluateOLS(t, OLSRequest{X: []float64{1, 2}, Y: []float64{1}, P: 2})
			So(small.Defined, ShouldBeFalse)
		})
	})
}

func TestCoefficientSNRNext(t *testing.T) {
	Convey("Given a defined coefficient and variance", t, func() {
		Convey("SNR is coefficient squared over variance", func() {
			So(evaluateSNR(t, CoefficientSNRPair{Coefficient: 2, Variance: 1}), ShouldEqual, 4)
		})

		Convey("undefined variance yields NaN, not zero", func() {
			So(math.IsNaN(evaluateSNR(t, CoefficientSNRPair{Coefficient: 2, Variance: 0})), ShouldBeTrue)
			So(math.IsNaN(evaluateSNR(t, CoefficientSNRPair{Coefficient: 2, Variance: math.NaN()})), ShouldBeTrue)
		})

		Convey("zero coefficient yields a valid zero SNR", func() {
			So(evaluateSNR(t, CoefficientSNRPair{Coefficient: 0, Variance: 1}), ShouldEqual, 0)
		})
	})
}

/*
collectReadings drives one Primitive over a slice of inputs and collects the
reading each arrival produced.
*/
func collectReadings[T any, U any](t *testing.T, operation core.Primitive, inputs []T) []U {
	t.Helper()

	var readings []U

	for index := range inputs {
		for out := range operation.Next(transport.NewValues(inputs[index]).Next(nil)) {
			readings = append(readings, *(*U)(out))
		}
	}

	if err := operation.Error(); err != nil {
		t.Fatalf("primitive evaluation: %v", err)
	}

	return readings
}
