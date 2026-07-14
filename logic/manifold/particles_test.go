package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func TestParticlesFromOscillators(t *testing.T) {
	Convey("Given post-step oscillator readback", t, func() {
		config := &pmanifold.Config{
			GridX:   64,
			GridY:   3,
			GridZ:   38,
			DomainX: 64,
			DomainY: 3,
			DomainZ: 38,
		}
		oscillators := []pmanifold.Oscillator{
			{
				Phase: 0.1, Omega: 1.2, Amplitude: 0.1,
				PosX: 8, PosY: 1, PosZ: 12,
				Heat: 0.4, VelX: 0.1, VelY: 0.2, VelZ: 0.3,
			},
			{
				Phase: 0.2, Omega: 1.4, Amplitude: 0.12,
				PosX: 16, PosY: 1, PosZ: 14,
				Heat: 0.5, VelX: 0.1, VelY: 0.1, VelZ: 0.2,
			},
			{
				Phase: 0.4, Omega: 1.7, Amplitude: 0.9,
				PosX: 40, PosY: 2, PosZ: 20,
				Heat: 1.1, VelX: 0.2, VelY: 0.1, VelZ: 0.4,
			},
		}

		Convey("It should publish whale carriers for amplitude standouts", func() {
			particles := particlesFromOscillators(config, oscillators)

			So(len(particles), ShouldEqual, 3)
			So(particles[0].Role, ShouldEqual, "carrier")
			So(particles[1].Role, ShouldEqual, "carrier")
			So(particles[2].Role, ShouldEqual, "whale_carrier")
			So(particles[2].CellX, ShouldEqual, float64(40))
			So(particles[2].CellZ, ShouldEqual, float64(20))
			So(particles[2].Speed, ShouldBeGreaterThan, 0)
		})
	})
}
