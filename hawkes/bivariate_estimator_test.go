package hawkes

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestBivariateEstimatorFit(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)

	convey.Convey("Given a self-exciting buy burst", t, func() {
		fit := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon)

		convey.Convey("It should recover subcritical excitation above baseline", func() {
			convey.So(fit.MuBuy, convey.ShouldBeGreaterThan, 0)
			convey.So(fit.Beta, convey.ShouldBeGreaterThan, 0)
			convey.So(fit.BuyIntensity, convey.ShouldBeGreaterThan, fit.MuBuy)
			convey.So(fit.SpectralRadius, convey.ShouldBeGreaterThan, 0)
			convey.So(fit.SpectralRadius, convey.ShouldBeLessThan, criticalBranch)
		})
	})

	convey.Convey("Given a sell-then-buy cascade", t, func() {
		sellEvents := burstEvents(start, 16, 55*time.Millisecond)
		cascadeBuys := make([]time.Time, 6)

		for index := range cascadeBuys {
			cascadeBuys[index] = sellEvents[index].Add(20 * time.Millisecond)
		}

		cascadeHorizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)
		cascadeStream := NewArrivalStream(cascadeBuys, sellEvents)
		fit := NewBivariateEstimator(BivariateFit{}).Fit(cascadeStream, cascadeHorizon)

		convey.Convey("It should prefer cross excitation over a diagonal-only fit", func() {
			convey.So(fit.MuBuy, convey.ShouldBeGreaterThan, 0)

			fittedLL := fit.LogLikelihood(cascadeStream, cascadeHorizon)
			noCross := BivariateFit{
				MuBuy:   fit.MuBuy,
				MuSell:  fit.MuSell,
				AlphaBB: fit.AlphaBB,
				AlphaSS: fit.AlphaSS,
				Beta:    fit.Beta,
			}
			noCrossLL := noCross.LogLikelihood(cascadeStream, cascadeHorizon)

			convey.So(fittedLL, convey.ShouldBeGreaterThan, noCrossLL)
			convey.So(fit.AlphaBS+fit.AlphaSB, convey.ShouldBeGreaterThan, 0)
		})
	})

	convey.Convey("Given a warm-start prior from an existing fit", t, func() {
		prior := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon)
		second := NewBivariateEstimator(prior).Fit(stream, horizon)

		convey.Convey("It should refit successfully", func() {
			convey.So(second.MuBuy, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestBivariateEstimatorMaximizeLikelihood(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 12, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(20*time.Millisecond), 10, 45*time.Millisecond)
	horizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)

	convey.Convey("Given a bivariate arrival stream", t, func() {
		context, ok := NewFitContext(stream, horizon)

		convey.So(ok, convey.ShouldBeTrue)

		estimator := NewBivariateEstimator(BivariateFit{})
		seeds := estimator.multiStartSeeds(context)

		convey.So(len(seeds), convey.ShouldBeGreaterThan, 0)

		convey.Convey("It should maximize likelihood from a seed", func() {
			fit := estimator.maximizeLikelihood(stream, horizon, context, seeds[0])

			convey.So(fit.MuBuy, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkBivariateEstimatorMaximizeLikelihood(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 12, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(20*time.Millisecond), 10, 45*time.Millisecond)
	horizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)
	context, _ := NewFitContext(stream, horizon)
	estimator := NewBivariateEstimator(BivariateFit{})
	seeds := estimator.multiStartSeeds(context)

	b.ReportAllocs()

	for b.Loop() {
		estimator.maximizeLikelihood(stream, horizon, context, seeds[0])
	}
}

func BenchmarkBivariateEstimatorFitWarmStart(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 32, 12, 25*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)
	prior := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon)

	b.ReportAllocs()

	for b.Loop() {
		if fit := NewBivariateEstimator(prior).Fit(stream, horizon); fit.MuBuy <= 0 {
			b.Fatal("expected fit")
		}
	}
}

func BenchmarkBivariateEstimatorFit(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 32, 12, 25*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)

	b.ReportAllocs()

	for b.Loop() {
		if fit := NewBivariateEstimator(BivariateFit{}).Fit(stream, horizon); fit.MuBuy <= 0 {
			b.Fatal("expected fit")
		}
	}
}
