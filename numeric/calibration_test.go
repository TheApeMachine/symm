package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestRegimeTargetShares(t *testing.T) {
	Convey("Given canonical target shares", t, func() {
		base := []float64{0.40, 0.30, 0.20, 0.10}

		Convey("Trending regimes increase the top-band target", func() {
			shares := RegimeTargetShares(base, perspectives.RegimeTrending)

			So(shares[len(shares)-1], ShouldBeGreaterThan, base[len(base)-1])
		})

		Convey("Choppy regimes increase the middle-band target", func() {
			shares := RegimeTargetShares(base, perspectives.RegimeChoppy)

			So(shares[len(shares)/2], ShouldBeGreaterThan, base[len(base)/2])
		})
	})
}

func TestRegimeBlend(t *testing.T) {
	Convey("Given a base blend", t, func() {
		Convey("Choppy regimes damp edge movement more", func() {
			So(RegimeBlend(0.3, perspectives.RegimeChoppy), ShouldBeGreaterThan, 0.3)
		})
	})
}

func TestEntropyTrustFromShares(t *testing.T) {
	Convey("Given category share mixes", t, func() {
		Convey("A dominant category yields high trust", func() {
			So(EntropyTrustFromShares([]float64{0.9, 0.05, 0.03, 0.02}), ShouldBeGreaterThan, 0.5)
		})

		Convey("A uniform mix yields low trust", func() {
			uniform := EntropyTrustFromShares([]float64{0.25, 0.25, 0.25, 0.25})
			skewed := EntropyTrustFromShares([]float64{0.9, 0.05, 0.03, 0.02})

			So(uniform, ShouldBeLessThan, skewed)
		})
	})
}

func TestBandCalibratorSeedFromObservations(t *testing.T) {
	Convey("Given prior observations", t, func() {
		classifier := adaptive.NewClassifier(
			[]float64{0.5, 2.0, 4.0},
			[]float64{0, 1, 2, 3},
			[]string{"a", "b", "c", "d"},
		)
		calibrator := NewBandCalibrator([]float64{0.25, 0.25, 0.25, 0.25}, 2000, 500, 100, 0)

		observations := make([]float64, 0, 120)

		for index := 0; index < 120; index++ {
			observations = append(observations, 0.4*float64(index%100)/100.0)
		}

		calibrator.SeedFromObservations(classifier, observations)

		Convey("It should refit before live traffic arrives", func() {
			telemetry := calibrator.Snapshot(classifier)

			So(telemetry.Calibrated, ShouldBeTrue)
			So(classifier.Upper()[0], ShouldBeLessThan, 0.5)
		})
	})
}

func BenchmarkEntropyTrustFromShares(b *testing.B) {
	shares := []float64{0.25, 0.25, 0.25, 0.25}

	for b.Loop() {
		_ = EntropyTrustFromShares(shares)
	}
}
