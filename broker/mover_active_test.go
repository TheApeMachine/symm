package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestActiveReasonVolumeAndChange(t *testing.T) {
	Convey("mover active rejects quiet 24h volume", t, func() {
		price := &Price{Mover: NewMover()}
		price.Mover.Apply("QUIET/USD", 1_000, 5.0)

		reason := price.ActiveReason("QUIET/USD")
		So(reason, ShouldContainSubstring, "24h volume")
		So(price.Active("QUIET/USD"), ShouldBeFalse)
	})

	Convey("mover active rejects flat day change after volume clears", t, func() {
		price := &Price{Mover: NewMover()}
		floor := system.Cfg.Risk.MinMoverVolumeUSD
		price.Mover.Apply("FLAT/USD", floor+1, 0.0)

		reason := price.ActiveReason("FLAT/USD")
		So(reason, ShouldContainSubstring, "day change")
		So(price.Active("FLAT/USD"), ShouldBeFalse)
	})

	Convey("mover active advances to spread after volume and change clear", t, func() {
		price := &Price{Mover: NewMover()}
		floor := system.Cfg.Risk.MinMoverVolumeUSD
		minChange := system.Cfg.Risk.MinMoverChangePct
		price.Mover.Apply("HOT/USD", floor+1, minChange+0.1)

		reason := price.ActiveReason("HOT/USD")
		So(reason, ShouldEqual, "no touch spread")
	})
}
