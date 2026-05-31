package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestEntryClearsFriction(t *testing.T) {
	convey.Convey("Given EntryEdgeMultiple and taker fees", t, func() {
		convey.Convey("It should reject scores below the friction-scaled floor", func() {
			convey.So(entryClearsFriction(
				0.5,
				config.System.EntryEdgeMultiple,
				config.System.TakerFeePct,
				config.System.TakerFeePct,
				0,
			), convey.ShouldBeFalse)
		})

		convey.Convey("It should accept scores that clear the floor", func() {
			convey.So(entryClearsFriction(
				4.0,
				config.System.EntryEdgeMultiple,
				config.System.TakerFeePct,
				config.System.TakerFeePct,
				12,
			), convey.ShouldBeTrue)
		})
	})

	convey.Convey("Given maker entry and taker exit fees", t, func() {
		convey.Convey("It should clear at a lower score than taker taker friction", func() {
			makerTaker := entryClearsFriction(2.1, 3, 0.25, 0.40, 0)
			takerTaker := entryClearsFriction(2.1, 3, 0.40, 0.40, 0)

			convey.So(makerTaker, convey.ShouldBeTrue)
			convey.So(takerTaker, convey.ShouldBeFalse)
		})
	})
}
