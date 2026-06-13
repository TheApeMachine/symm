package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
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
	Convey("Given score inertia config", t, func() {
		originalEffort := viper.GetInt("signals.score_inertia.effort")
		originalMinObs := viper.GetInt("regime.baseline.min_obs")

		t.Cleanup(func() {
			viper.Set("signals.score_inertia.effort", originalEffort)
			viper.Set("regime.baseline.min_obs", originalMinObs)
		})

		Convey("It should prefer signals.score_inertia.effort when set", func() {
			viper.Set("signals.score_inertia.effort", 7)
			viper.Set("regime.baseline.min_obs", 16)

			So(resolveScoreInertiaEffort(), ShouldEqual, 7)
		})

		Convey("It should fall back to regime.baseline.min_obs", func() {
			viper.Set("signals.score_inertia.effort", 0)
			viper.Set("regime.baseline.min_obs", 16)

			So(resolveScoreInertiaEffort(), ShouldEqual, 16)
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
