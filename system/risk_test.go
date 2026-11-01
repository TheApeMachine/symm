package system_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestRisk_Apply(t *testing.T) {
	Convey("Given a risk config", t, func() {
		risk := system.NewRisk()

		Convey("It patches known floors from a gate-lab command", func() {
			risk.Apply(map[string]any{
				"type":                   "risk",
				"min_mover_volume_usd":   50_000.0,
				"min_mover_change_pct":   1.25,
				"max_entry_spread_pct":   0.75,
				"min_touch_bid_cash":     250.0,
				"min_backing_ratio":      0.4,
				"min_tape_trades":        3.0,
				"max_entry_open_drag_pct": 1.1,
			})

			So(risk.MinMoverVolumeUSD, ShouldEqual, 50_000.0)
			So(risk.MinMoverChangePct, ShouldEqual, 1.25)
			So(risk.MaxEntrySpreadPct, ShouldEqual, 0.75)
			So(risk.MinTouchBidCash, ShouldEqual, 250.0)
			So(risk.MinBackingRatio, ShouldEqual, 0.4)
			So(risk.MinTapeTrades, ShouldEqual, 3)
			So(risk.MaxEntryOpenDragPct, ShouldEqual, 1.1)
		})
	})
}
