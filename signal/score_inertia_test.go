package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/logic"
)

func TestDirectionalScoreInertiaApply(t *testing.T) {
	Convey("Given directional score inertia", t, func() {
		Convey("It should publish the first raw value immediately", func() {
			inertia := DirectionalScoreInertia{}

			So(inertia.Apply(0.2, 3), ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("It should require effort before moving up from rest", func() {
			inertia := DirectionalScoreInertia{}

			_ = inertia.Apply(0.2, 3)
			So(inertia.Apply(0.3, 3), ShouldAlmostEqual, 0.2, 1e-9)
			So(inertia.Apply(0.31, 3), ShouldAlmostEqual, 0.2, 1e-9)
			So(inertia.Apply(0.32, 3), ShouldAlmostEqual, 0.32, 1e-9)
		})

		Convey("It should move freely once upward momentum is established", func() {
			inertia := DirectionalScoreInertia{}

			_ = inertia.Apply(0.2, 2)
			_ = inertia.Apply(0.25, 2)
			_ = inertia.Apply(0.27, 2)
			So(inertia.Apply(0.4, 2), ShouldAlmostEqual, 0.4, 1e-9)
		})

		Convey("It should require effort before reversing downward", func() {
			inertia := DirectionalScoreInertia{}

			_ = inertia.Apply(0.2, 2)
			_ = inertia.Apply(0.25, 2)
			_ = inertia.Apply(0.27, 2)
			_ = inertia.Apply(0.4, 2)

			So(inertia.Apply(0.35, 2), ShouldAlmostEqual, 0.4, 1e-9)
			So(inertia.Apply(0.33, 2), ShouldAlmostEqual, 0.33, 1e-9)
		})

		Convey("It should reset effort when raw matches published", func() {
			inertia := DirectionalScoreInertia{}

			_ = inertia.Apply(0.2, 2)
			_ = inertia.Apply(0.25, 2)
			So(inertia.Apply(0.2, 2), ShouldAlmostEqual, 0.2, 1e-9)
			So(inertia.Apply(0.3, 2), ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("It should reset effort when desire direction alternates", func() {
			inertia := DirectionalScoreInertia{}

			_ = inertia.Apply(0.5, 2)
			_ = inertia.Apply(0.6, 2)
			_ = inertia.Apply(0.4, 2)
			So(inertia.Apply(0.6, 2), ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestResolveScoreInertiaEffort(t *testing.T) {
	Convey("Given derived regime sizing", t, func() {
		testconfig.SeedCompactRegime()
		regimeSpec, err := config.DerivedRegimeSpec()

		So(err, ShouldBeNil)

		Convey("It should match regime min samples", func() {
			So(resolveScoreInertiaEffort(), ShouldEqual, regimeSpec.MinSamples)
		})
	})
}

func BenchmarkDirectionalScoreInertiaApply(b *testing.B) {
	inertia := DirectionalScoreInertia{}
	rawValues := []float64{0.2, 0.25, 0.27, 0.4, 0.35, 0.33, 0.31, 0.29}
	effortThreshold := 4

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = inertia.Apply(rawValues[index%len(rawValues)], effortThreshold)
	}
}

func BenchmarkSystemApplyScoreInertia(b *testing.B) {
	system := &System{scoreInertiaEffort: 4}
	state := &symbolScoreInertia{}
	measurement := logic.Measurement{
		Symbol:     "BTC/USD",
		Confidence: 0.2,
		Surprise:   0.2,
	}
	rawValues := []float64{0.2, 0.25, 0.27, 0.4, 0.35, 0.33, 0.31, 0.29}

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		measurement.Confidence = rawValues[index%len(rawValues)]
		measurement.Surprise = rawValues[(index+1)%len(rawValues)]
		_ = system.applyScoreInertia(measurement, state)
	}
}

func BenchmarkSystemScoreInertiaLookup(b *testing.B) {
	system := &System{scoreInertiaEffort: 4}
	symbol := "BTC/USD"
	system.scoreInertiaFor(symbol)

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = system.scoreInertiaFor(symbol)
	}
}
