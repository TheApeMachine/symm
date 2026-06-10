package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/telemetry"
)

func TestAdaptationControllerAlpha(t *testing.T) {
	Convey("Given baseline bounds and surprise readings", t, func() {
		configureRegimeViper()

		controller, err := NewAdaptationController()

		So(err, ShouldBeNil)

		surpriseIndex := telemetry.SharedSurpriseIndex()
		savedRatios := surpriseIndex.SnapshotRatios()

		t.Cleanup(func() {
			surpriseIndex.RestoreRatios(savedRatios)
		})

		surpriseIndex.Reset()
		telemetry.RecordSurpriseRatio("fluid", 4, 2)

		Convey("It should raise alpha when surprise spikes", func() {
			So(controller.Alpha(), ShouldEqual, 0.25)
		})
	})
}

func TestAdaptationControllerContagionWindows(t *testing.T) {
	Convey("Given a warmed market vol baseline", t, func() {
		configureRegimeViper()

		controller, err := NewAdaptationController()

		So(err, ShouldBeNil)

		for range 64 {
			controller.ObserveRegimeSamples([]float64{0.001}, nil)
		}

		_, _, baselineSlow := controller.ContagionWindows()

		controller.ObserveRegimeSamples([]float64{0.008}, nil)

		fastWindow, mediumWindow, slowWindow := controller.ContagionWindows()

		Convey("It should widen windows when realized vol rises", func() {
			So(slowWindow, ShouldBeGreaterThan, baselineSlow)
			So(mediumWindow, ShouldEqual, slowWindow/2)
			So(fastWindow, ShouldEqual, slowWindow/8)
		})
	})
}

func BenchmarkAdaptationControllerObserveRegimeSamples(b *testing.B) {
	viper.Set("regime.baseline.alpha_min", 0.01)
	viper.Set("regime.baseline.alpha_max", 0.25)
	viper.Set("regime.baseline.min_obs", 4)
	viper.Set("regime.baseline.trend_sigma", 1.25)
	viper.Set("regime.baseline.strong_trend_sigma", 2.5)
	viper.Set("regime.baseline.vol_floor_sigma", 3.0)
	viper.Set("regime.baseline.vol_scale_floor", 0.000001)
	viper.Set("regime.baseline.seed_vol_scale", 0.01)
	viper.Set("signals.causal.contagion_window_slow_max", 128)
	viper.Set("signals.causal.contagion_window_slow_min", 16)

	controller, err := NewAdaptationController()

	if err != nil {
		b.Fatal(err)
	}

	volatilities := []float64{0.01, 0.012, 0.011}
	trendScores := []float64{1.5, 1.8, 1.6}

	b.ReportAllocs()

	for b.Loop() {
		controller.ObserveRegimeSamples(volatilities, trendScores)
	}
}
