package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestWirePackets(t *testing.T) {
	Convey("Given a full manifold state", t, func() {
		state := State{
			Source:                "manifold",
			Symbol:                "BTC/USD",
			At:                    time.Unix(1, 0).UTC(),
			OscillatorCount:       2,
			SharedOscillatorCount: 2,
			Grid:                  pfluid.Grid{X: 8, Y: 4, Z: 6, Spacing: 0.1},
			Reading: pfluid.Reading{
				PressureGradX: 1, PressureGradY: 2, PressureGradZ: 3,
				Divergence: 4, CoherenceMag2: 5, GuidanceSpeed: 6, ViscosityProxy: 7,
			},
			Rho:          [][]float64{{0.1, 0.2}, {0.3, 0.4}},
			PsiMag2:      [][]float64{{0.2, 0.1}, {0.4, 0.3}},
			GuidanceVelX: [][]float64{{0.3, 0.0}, {0.0, 0.3}},
			GuidanceVelZ: [][]float64{{0.4, 0.0}, {0.0, 0.4}},
			Particles: []Particle{{
				Role: "particle", CellX: 1, CellY: 2, CellZ: 3,
				Phase: 0.5, Omega: 9, Amplitude: 1.5, Heat: 0.01,
				VelX: 0.1, VelY: 0.2, VelZ: 0.3, Speed: 0.4,
				SpatialTokenID: 99,
			}},
			Wave: []pfluid.WaveMode{{
				Omega: 1, Real: 0.2, Imaginary: 0.3, Linewidth: 0.4,
			}},
			PhaseReady: true,
		}

		Convey("It emits meta without lattices and four binary planes", func() {
			field, lattices, particles, wave := state.WirePackets()
			So(field.Symbol, ShouldEqual, "BTC/USD")
			So(field.Grid.X, ShouldEqual, uint32(8))
			So(field.Grid.Z, ShouldEqual, uint32(6))
			So(field.Reading.PressureGradX, ShouldEqual, 1.0)
			So(field.PhaseReady, ShouldBeTrue)
			So(lattices, ShouldHaveLength, 4)

			for index, want := range []string{
				"manifold_rho", "manifold_psi",
				"manifold_guidance_x", "manifold_guidance_z",
			} {
				key, known := BinaryCacheKey(lattices[index])
				So(known, ShouldBeTrue)
				So(key, ShouldEqual, want)
			}
			So(particles.Particles, ShouldHaveLength, 1)
			So(particles.Particles[0].CellX, ShouldEqual, 1)
			So(wave.Wave, ShouldHaveLength, 1)
			So(wave.Wave[0].Real, ShouldEqual, float32(0.2))
		})
	})
}
