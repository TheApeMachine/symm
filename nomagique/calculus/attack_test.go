package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNewAttack(t *testing.T) {
	Convey("Given a clock and shape", t, func() {
		clock := temporal.NewClock(types.NewInput(0.0), types.NewInput(1.0))

		Convey("When NewAttack is called", func() {
			attack := NewAttack(clock, shape)

			Convey("Then the returned Attack should have the correct clock and shape", func() {
				So(attack.clock, ShouldEqual, clock)
				So(attack.shape, ShouldEqual, shape)
			})
		})
	})
}
