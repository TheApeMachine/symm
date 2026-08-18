package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParticleCount(t *testing.T) {
	Convey("Setup", t, func() {
		instance := NewSolver()

		Convey("When counting the particles", func() {
			count := m.ParticleCount()

			Convey("Then the count should be correct", func() {
				So(count, ShouldEqual, 3)
			})
		})
	})
}
