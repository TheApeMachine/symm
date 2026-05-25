package hawkes

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func sampleFit() BivariateFit {
	return BivariateFit{
		MuBuy:          1,
		MuSell:         1,
		AlphaBB:        2,
		AlphaBS:        0.5,
		AlphaSB:        0.2,
		AlphaSS:        0.3,
		Beta:           4,
		BuyIntensity:   3,
		SellIntensity:  1.2,
		SpectralRadius: 0.5,
	}
}

func TestApplyExcitationCalibration(t *testing.T) {
	base := sampleFit()

	convey.Convey("Given neutral calibration", t, func() {
		convey.Convey("It should leave the fit unchanged", func() {
			calibrated := applyExcitationCalibration(base, 1)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, base.AlphaBB)
			convey.So(calibrated.BuyIntensity, convey.ShouldEqual, base.BuyIntensity)
		})
	})

	convey.Convey("Given overconfident feedback calibration", t, func() {
		convey.Convey("It should scale buy-side excitation parameters down", func() {
			calibrated := applyExcitationCalibration(base, 0.5)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, base.AlphaBB*0.5)
			convey.So(calibrated.AlphaBS, convey.ShouldEqual, base.AlphaBS*0.5)
			convey.So(calibrated.BuyIntensity, convey.ShouldAlmostEqual, 2, 0.0001)
		})
	})

	convey.Convey("Given repeated misses", t, func() {
		convey.Convey("It should zero buy-side excitation in the prior", func() {
			calibrated := applyExcitationCalibration(base, 0)

			convey.So(calibrated.AlphaBB, convey.ShouldEqual, 0)
			convey.So(calibrated.AlphaBS, convey.ShouldEqual, 0)
			convey.So(calibrated.BuyIntensity, convey.ShouldEqual, base.MuBuy)
		})
	})
}

func TestApplyPredictionFeedback(t *testing.T) {
	hawkesSignal := &Hawkes{
		calibrationParams: engine.DefaultCalibrationParams(),
		states:            make(map[string]*HawkesSymbol),
	}

	convey.Convey("Given an overconfident settled forecast", t, func() {
		sym := hawkesSignal.state("PUMP/EUR")
		sym.fit = sampleFit()
		sym.hasFit = true

		hawkesSignal.Feedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "PUMP/EUR",
			PredictedReturn: 0.1,
			ActualReturn:    0.05,
		})

		convey.Convey("It should lower excitation calibration", func() {
			prior := applyExcitationCalibration(sym.fit, sym.calibrator.Scale())

			convey.So(sym.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
			convey.So(prior.AlphaBB, convey.ShouldAlmostEqual, sym.fit.AlphaBB*0.5, 0.0001)
		})
	})
}

func BenchmarkApplyExcitationCalibration(b *testing.B) {
	base := sampleFit()

	b.ReportAllocs()

	for b.Loop() {
		applyExcitationCalibration(base, 0.75)
	}
}
