package statistic_test

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestMean(t *testing.T) {
	Convey("Mean computes the running arithmetic mean", t, func() {
		mean := statistic.NewMean()
		out := tests.CollectSeq[float64](mean.Next(transport.NewValues(1.0, 2.0, 3.0, 4.0, 5.0).Next(nil)))

		So(mean.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 5)
		So(out[0], ShouldEqual, 1.0)
		So(out[1], ShouldEqual, 1.5)
		So(out[2], ShouldEqual, 2.0)
		So(out[3], ShouldEqual, 2.5)
		So(out[4], ShouldEqual, 3.0)
	})
}

func TestMedian(t *testing.T) {
	Convey("Median averages the central order statistics of a run", t, func() {
		med := statistic.NewMedian()
		out := tests.CollectSeq[float64](med.Next(transport.NewValues(9.0, 1.0, 5.0).Next(nil)))

		So(med.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldEqual, 5.0)

		med2 := statistic.NewMedian()
		out2 := tests.CollectSeq[float64](med2.Next(transport.NewValues(9.0, 1.0, 5.0, 3.0).Next(nil)))
		So(out2[0], ShouldEqual, 4.0)

		medEmpty := statistic.NewMedian()
		outEmpty := tests.CollectSeq[float64](medEmpty.Next(transport.NewValues[float64]().Next(nil)))
		So(len(outEmpty), ShouldEqual, 0)
		So(errors.Is(medEmpty.Error(), core.ErrShape), ShouldBeTrue)
	})
}

func TestStandardize(t *testing.T) {
	Convey("Standardize with fixed parameters", t, func() {
		std := statistic.NewStandardize(10.0, 2.0)
		out := tests.CollectSeq[float64](std.Next(transport.NewValues(10.0, 12.0, 8.0).Next(nil)))

		So(std.Error(), ShouldBeNil)
		So(out[0], ShouldEqual, 0.0)
		So(out[1], ShouldEqual, 1.0)
		So(out[2], ShouldEqual, -1.0)
	})

	Convey("Standardize with per-arrival input", t, func() {
		std := statistic.NewStandardize()
		in := []statistic.StandardizeInput{
			{Value: 15, Center: 10, Scale: 5},
			{Value: 5, Center: 10, Scale: 5},
		}
		out := tests.CollectSeq[float64](std.Next(transport.NewValues(in...).Next(nil)))

		So(std.Error(), ShouldBeNil)
		So(out[0], ShouldEqual, 1.0)
		So(out[1], ShouldEqual, -1.0)
	})
}

func TestEstimator(t *testing.T) {
	Convey("Estimator computes running Welford moments", t, func() {
		est := statistic.NewEstimator()
		out := tests.CollectSeq[statistic.MomentReading](est.Next(transport.NewValues(2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0).Next(nil)))

		So(est.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 8)
		last := out[len(out)-1]
		So(last.Count, ShouldEqual, 8)
		So(last.Mean, ShouldEqual, 5.0)
		So(last.Variance, ShouldAlmostEqual, 32.0/7.0, 1e-12)
		So(last.Dispersion, ShouldAlmostEqual, math.Sqrt(32.0/7.0), 1e-12)
	})

	Convey("PredictiveInflation computes sqrt(1 + 1/count)", t, func() {
		op := statistic.NewPredictiveInflation()
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(1.0, 3.0).Next(nil)))
		So(op.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, math.Sqrt(2.0), 1e-9)
		So(out[1], ShouldAlmostEqual, math.Sqrt(4.0/3.0), 1e-9)
	})

	Convey("RMS computes sqrt(sum(x^2)/n)", t, func() {
		op := statistic.NewRMS()
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(3.0, 4.0).Next(nil)))
		So(op.Error(), ShouldBeNil)
		So(out[0], ShouldEqual, 3.0)
		// (9 + 16) / 2 = 12.5, sqrt(12.5) ≈ 3.5355
		So(out[1], ShouldAlmostEqual, math.Sqrt(12.5), 1e-9)
	})

	Convey("SamplingVariance applies specificity debt floor", t, func() {
		op := statistic.NewSamplingVariance()
		in := statistic.SamplingVarianceInput{
			Depth:         2.0,
			ContextLength: 4.0,
			Support:       10.0,
			Variance:      3.0,
		}
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(in).Next(nil)))
		So(op.Error(), ShouldBeNil)
		// floor = 10 / (1 + 2) = 10/3. variance / floor = 3 / (10/3) = 0.9
		So(out[0], ShouldAlmostEqual, 0.9, 1e-9)
	})

	Convey("ResidualSpan tracks min, max, span", t, func() {
		op := statistic.NewResidualSpan()
		in1 := statistic.ResidualSpanInput{Count: 0, Residual: 5.0}
		in2 := statistic.ResidualSpanInput{Count: 1, Minimum: 5.0, Maximum: 5.0, Residual: 2.0}
		in3 := statistic.ResidualSpanInput{Count: 2, Minimum: 2.0, Maximum: 5.0, Residual: 8.0}
		out := tests.CollectSeq[statistic.ResidualSpanResult](op.Next(transport.NewValues(in1, in2, in3).Next(nil)))
		So(op.Error(), ShouldBeNil)
		So(out[2].Minimum, ShouldEqual, 2.0)
		So(out[2].Maximum, ShouldEqual, 8.0)
		So(out[2].Span, ShouldEqual, 6.0)
	})
}
