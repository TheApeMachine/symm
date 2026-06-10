package manifold

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/physics"
)

func TestDisplayCarriersFallbackFiniteOscillator(t *testing.T) {
	convey.Convey("Given a non-finite solver readback", t, func() {
		field := &Field{}
		fallback := physics.Oscillator{
			PosX: 1,
			PosY: 2,
			PosZ: 3,
			VelX: 0.1,
		}

		symbolCarriers := []fieldCarrier{{
			role:       "symbol",
			symbol:     "XBT/USD",
			oscillator: fallback,
		}}
		solverCarriers := []fieldCarrier{{
			role:   "symbol",
			symbol: "XBT/USD",
		}}
		readOscillators := []physics.Oscillator{{
			PosX: math.NaN(),
			PosY: 2,
			PosZ: 3,
		}}

		convey.Convey("It should keep the pre-step carrier state", func() {
			display := field.displayCarriers(symbolCarriers, solverCarriers, readOscillators)

			convey.So(len(display), convey.ShouldEqual, 1)
			convey.So(display[0].oscillator.PosX, convey.ShouldEqual, 1)
			convey.So(oscillatorStateFinite(display[0].oscillator), convey.ShouldBeTrue)
		})
	})
}
