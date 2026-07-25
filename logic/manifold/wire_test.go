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
			Display:               []byte{10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 100, 110, 120, 255},
			DisplayWidth:          2,
			DisplayHeight:         2,
			RhoOccupied:           3,
			PsiOccupied:           4,
			RhoMax:                1.5,
			PsiMax:                2.5,
			Reading: pfluid.Reading{
				PressureGradX: 1, PressureGradY: 2, PressureGradZ: 3,
				Divergence: 4, CoherenceMag2: 5, GuidanceSpeed: 6, ViscosityProxy: 7,
			},
			Wave: []pfluid.WaveMode{{
				Omega: 1, Real: 0.2, Imaginary: 0.3, Linewidth: 0.4,
			}},
			PhaseReady: true,
		}

		Convey("It emits meta and one composited display texture", func() {
			field, lattices, wave := state.WirePackets()
			So(field.Symbol, ShouldEqual, "BTC/USD")
			So(field.OscillatorCount, ShouldEqual, 2)
			So(field.RhoOccupied, ShouldEqual, 3)
			So(field.PsiOccupied, ShouldEqual, 4)
			So(field.RhoMax, ShouldEqual, 1.5)
			So(field.PsiMax, ShouldEqual, 2.5)
			So(field.Grid.X, ShouldEqual, uint32(8))
			So(field.Grid.Z, ShouldEqual, uint32(6))
			So(field.Reading.PressureGradX, ShouldEqual, 1.0)
			So(field.PhaseReady, ShouldBeTrue)
			So(lattices, ShouldHaveLength, 1)
			key, known := BinaryCacheKey(lattices[0])
			So(known, ShouldBeTrue)
			So(key, ShouldEqual, "manifold_display")
			So(wave.Wave, ShouldHaveLength, 1)
			So(wave.Wave[0].Real, ShouldEqual, float32(0.2))
		})
	})
}
