package hawkes

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func bivariateLogLikelihoodReference(
	stream ArrivalStream,
	horizon time.Time,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) float64 {
	marked := stream.Marked()

	if len(marked) == 0 {
		return math.Inf(-1)
	}

	span := stream.Span(horizon)

	if span <= 0 {
		return math.Inf(-1)
	}

	buySoFar := make([]time.Time, 0, len(stream.BuyTimes()))
	sellSoFar := make([]time.Time, 0, len(stream.SellTimes()))
	logSum := 0.0

	for _, event := range marked {
		var lambda float64

		switch event.side {
		case sideBuy:
			partial := NewArrivalStream(buySoFar, sellSoFar)
			lambda = partial.intensityAt(
				event.at,
				muBuy, alphaBB, alphaBS, beta,
			)
			buySoFar = append(buySoFar, event.at)
		case sideSell:
			partial := NewArrivalStream(buySoFar, sellSoFar)
			lambda = partial.sellIntensityAt(
				event.at,
				muSell, alphaSB, alphaSS, beta,
			)
			sellSoFar = append(sellSoFar, event.at)
		}

		if lambda <= 0 {
			return math.Inf(-1)
		}

		logSum += math.Log(lambda)
	}

	candidate := BivariateFit{
		MuBuy:   muBuy,
		MuSell:  muSell,
		AlphaBB: alphaBB,
		AlphaBS: alphaBS,
		AlphaSB: alphaSB,
		AlphaSS: alphaSS,
		Beta:    beta,
	}
	compensator := candidate.compensator(stream, horizon, span)

	return logSum - compensator
}

func TestBivariateFitLogLikelihood(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)

	convey.Convey("Given a fitted bivariate Hawkes process", t, func() {
		fit := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon)

		convey.So(fit.MuBuy, convey.ShouldBeGreaterThan, 0)

		convey.Convey("It should match the event-wise reference likelihood", func() {
			incremental := fit.LogLikelihood(stream, horizon)
			reference := bivariateLogLikelihoodReference(
				stream, horizon,
				fit.MuBuy, fit.MuSell,
				fit.AlphaBB, fit.AlphaBS, fit.AlphaSB, fit.AlphaSS, fit.Beta,
			)

			convey.So(math.Abs(incremental-reference), convey.ShouldBeLessThan, 1e-9)
		})
	})
}

func TestBivariateFitLogLikelihoodGradient(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)
	fit := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon)

	convey.Convey("Given a fitted bivariate Hawkes process", t, func() {
		convey.So(fit.MuBuy, convey.ShouldBeGreaterThan, 0)

		_, analytical, ok := fit.LogLikelihoodGradient(stream, horizon)

		convey.So(ok, convey.ShouldBeTrue)

		convey.Convey("It should match numerical partial derivatives", func() {
			numerical := numericalLogLikelihoodGradient(t, fit, stream, horizon, 1e-6)

			for index, label := range []string{
				"muBuy", "muSell", "alphaBB", "alphaBS", "alphaSB", "alphaSS", "beta",
			} {
				convey.Printf("gradient %s analytical=%v numerical=%v\n",
					label, analytical[index], numerical[index])
				convey.So(
					math.Abs(analytical[index]-numerical[index]),
					convey.ShouldBeLessThan,
					5e-3,
				)
			}
		})
	})
}

func numericalLogLikelihoodGradient(
	t *testing.T,
	fit BivariateFit,
	stream ArrivalStream,
	horizon time.Time,
	step float64,
) [7]float64 {
	t.Helper()

	baseLL, _, ok := fit.LogLikelihoodGradient(stream, horizon)

	if !ok {
		t.Fatal("expected base likelihood")
	}

	gradient := [7]float64{}
	perturb := func(mutate func(*BivariateFit)) float64 {
		candidate := fit
		mutate(&candidate)
		value, _, ok := candidate.LogLikelihoodGradient(stream, horizon)

		if !ok {
			return math.Inf(-1)
		}

		return value
	}

	gradient[0] = (perturb(func(candidate *BivariateFit) {
		candidate.MuBuy += step
	}) - baseLL) / step
	gradient[1] = (perturb(func(candidate *BivariateFit) {
		candidate.MuSell += step
	}) - baseLL) / step
	gradient[2] = (perturb(func(candidate *BivariateFit) {
		candidate.AlphaBB += step
	}) - baseLL) / step
	gradient[3] = (perturb(func(candidate *BivariateFit) {
		candidate.AlphaBS += step
	}) - baseLL) / step
	gradient[4] = (perturb(func(candidate *BivariateFit) {
		candidate.AlphaSB += step
	}) - baseLL) / step
	gradient[5] = (perturb(func(candidate *BivariateFit) {
		candidate.AlphaSS += step
	}) - baseLL) / step
	gradient[6] = (perturb(func(candidate *BivariateFit) {
		candidate.Beta += step
	}) - baseLL) / step

	return gradient
}

func TestBivariateFitAsymmetry(t *testing.T) {
	convey.Convey("Given buy-side intensity dominance", t, func() {
		fit := BivariateFit{BuyIntensity: 3, SellIntensity: 1, MuBuy: 1, SpectralRadius: 0.4}

		convey.Convey("It should report positive buy asymmetry", func() {
			convey.So(fit.Asymmetry(false), convey.ShouldBeGreaterThan, 0)
		})

		convey.Convey("It should report zero buy asymmetry after sell takeover", func() {
			fit.SellIntensity = 4

			convey.So(fit.Asymmetry(false), convey.ShouldEqual, 0)
		})
	})
}

func TestBivariateFitExcitationConfidence(t *testing.T) {
	convey.Convey("Given critical branching", t, func() {
		fit := BivariateFit{
			BuyIntensity:   4,
			MuBuy:          1,
			SpectralRadius: 1.05,
		}

		convey.Convey("It should reject confidence", func() {
			convey.So(fit.ExcitationConfidence(0.5, 1, false), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given intensity below the baseline fence", t, func() {
		fit := BivariateFit{
			BuyIntensity:   2,
			MuBuy:          1,
			SpectralRadius: 0.4,
		}

		convey.Convey("It should reject confidence", func() {
			convey.So(fit.ExcitationConfidence(0.5, 3, false), convey.ShouldEqual, 0)
		})
	})
}

func TestBivariateFitComputeSpectralRadius(t *testing.T) {
	convey.Convey("Given subcritical branch ratios", t, func() {
		radius := BivariateFit{
			AlphaBB: 0.2,
			AlphaBS: 0.05,
			AlphaSB: 0.05,
			AlphaSS: 0.15,
			Beta:    1,
		}.computeSpectralRadius()

		convey.Convey("It should stay below critical branching", func() {
			convey.So(radius, convey.ShouldBeGreaterThan, 0)
			convey.So(radius, convey.ShouldBeLessThan, criticalBranch)
		})
	})
}

func TestBivariateFitCalibrated(t *testing.T) {
	base := sampleFit()

	convey.Convey("Given neutral calibration", t, func() {
		convey.Convey("It should leave the fit unchanged", func() {
			calibrated := base.Calibrated(1)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, base.AlphaBB)
			convey.So(calibrated.BuyIntensity, convey.ShouldEqual, base.BuyIntensity)
		})
	})

	convey.Convey("Given overconfident feedback calibration", t, func() {
		convey.Convey("It should scale buy-side excitation parameters down", func() {
			calibrated := base.Calibrated(0.5)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, base.AlphaBB*0.5)
			convey.So(calibrated.AlphaBS, convey.ShouldEqual, base.AlphaBS*0.5)
			convey.So(calibrated.BuyIntensity, convey.ShouldAlmostEqual, 2, 0.0001)
		})
	})

	convey.Convey("Given repeated misses", t, func() {
		convey.Convey("It should zero buy-side excitation in the prior", func() {
			calibrated := base.Calibrated(0)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, 0)
			convey.So(calibrated.AlphaBS, convey.ShouldEqual, 0)
			convey.So(calibrated.BuyIntensity, convey.ShouldEqual, base.MuBuy)
		})
	})
}

func BenchmarkBivariateFitCalibrated(b *testing.B) {
	base := sampleFit()

	b.ReportAllocs()

	for b.Loop() {
		base.Calibrated(0.75)
	}
}
