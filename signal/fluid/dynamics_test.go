package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidDynamicsIcebergScore(t *testing.T) {
	Convey("Given balanced add and execute rates at the touch", t, func() {
		dynamics := fluidDynamics{}
		dynamics.recordSourceBalance(10, 10)
		dynamics.recordSourceBalance(12, 11)
		dynamics.recordSourceBalance(9, 9)
		dynamics.recordSourceBalance(8, 8)

		score := dynamics.icebergScore(10, 10)

		Convey("It should score hidden absorption when rates balance", func() {
			So(score, ShouldEqual, 10)
		})
	})
}
