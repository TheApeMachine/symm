package manifold

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
TestDatasetStep pins the projection contract.

The rule that matters: Kraken sends Level-3 ONE SIDE AT A TIME. A projection
that needs both sides of a touch before it can place a particle places none at
all, because the two sides are never in the same message. Every assertion here
uses one-sided messages for that reason, and the tape replay below asserts the
whole stream produces particles rather than silence.
*/
/*
TestValidParticle pins the data-arrival gate: an order only becomes a particle
when every field it carries is determined. Zero, NaN and Inf seeds are dropped
before they reach the resident domain.
*/
func TestValidParticle(t *testing.T) {
	Convey("Given a particle whose energy would be non-positive or non-finite", t, func() {
		Convey("A zero-energy particle is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{0},
				Mass:   []float32{0},
				Amp:    []float32{0},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("A NaN energy is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{float32(math.NaN())},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("An infinite energy is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{float32(math.Inf(1))},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("A NaN coordinate is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{1},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{float32(math.NaN()), 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})
	})

	Convey("Given a fully determined particle", t, func() {
		state := sensorium.State{
			N:      1,
			Energy: []float32{1},
			Mass:   []float32{1},
			Amp:    []float32{1},
			Phase:  []float32{1},
			Omega:  []float32{1},
			Pos:    []float32{0.5, 0.5, 0.5},
			Vel:    []float32{0, 0, 0},
			Heat:   []float32{0},
		}

		Convey("It passes the gate", func() {
			So(validParticle(&state), ShouldBeTrue)
		})
	})
}

/*
TestSymbolToken pins that a symbol's particle-token space is derived from the
symbol itself, so two symbols cannot silently share one token space.
*/
func TestSymbolToken(t *testing.T) {
	Convey("Given distinct symbols", t, func() {
		So(symbolToken("BTC/USD"), ShouldNotEqual, symbolToken("ETH/USD"))

		Convey("The same symbol always resolves to the same token", func() {
			So(symbolToken("BTC/USD"), ShouldEqual, symbolToken("BTC/USD"))
		})

		Convey("Every token fits the packed index space", func() {
			So(symbolToken("BTC/USD"), ShouldBeLessThanOrEqualTo, symbolIndexMask)
		})
	})
}
