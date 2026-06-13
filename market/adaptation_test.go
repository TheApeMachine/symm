package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
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
			So(controller.Alpha(), ShouldEqual, controller.config.AlphaMax)
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
	testconfig.SeedCompactRegime()

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
