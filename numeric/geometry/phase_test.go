package geometry

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewPhase(t *testing.T) {
	t.Parallel()

	Convey("NewPhase returns a non-nil Phase", t, func() {
		So(NewPhase(), ShouldNotBeNil)
	})
}

func TestPhaseCoupling(t *testing.T) {
	t.Parallel()

	Convey("Given a Phase", t, func() {
		phase := NewPhase()

		Convey("when geometric mean is below magEps, It should return 0", func() {
			So(phase.Coupling(0.001, 0.001), ShouldEqual, 0)
		})

		Convey("when both sides are strong and same sign, It should return +1", func() {
			So(phase.Coupling(2, 2), ShouldAlmostEqual, 1, 1e-9)
		})

		Convey("when signs oppose, It should return -1", func() {
			So(phase.Coupling(2, -2), ShouldAlmostEqual, -1, 1e-9)
		})
	})
}

func TestPhaseVelocity(t *testing.T) {
	t.Parallel()

	Convey("Velocity is the first difference of surprisal means", t, func() {
		phase := NewPhase()

		So(phase.Velocity(1.5, 1), ShouldAlmostEqual, 0.5, 1e-9)
		So(phase.Velocity(0.25, 1), ShouldAlmostEqual, -0.75, 1e-9)
	})
}

func BenchmarkPhaseCoupling(b *testing.B) {
	phase := NewPhase()

	b.ResetTimer()

	for b.Loop() {
		_ = phase.Coupling(1.7, -0.9)
	}
}

func BenchmarkPhaseVelocity(b *testing.B) {
	phase := NewPhase()

	b.ResetTimer()

	for b.Loop() {
		_ = phase.Velocity(3.3, 1.1)
	}
}
