package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSurprisalVelocityCouple(t *testing.T) {
	Convey("SurprisalVelocityCouple", t, func() {
		So(SurprisalVelocityCouple(0.5, 0.5), ShouldBeGreaterThan, 0)
		So(SurprisalVelocityCouple(0.001, 0.001), ShouldEqual, 0)
	})
}

func BenchmarkSurprisalVelocityCouple(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		_ = SurprisalVelocityCouple(0.37, -0.21)
	}
}
