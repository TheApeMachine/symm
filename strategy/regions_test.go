package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
)

func TestRegions(t *testing.T) {
	Convey("Regions appends lit rows onto the tick for every symbol", t, func() {
		tick := data.NewValue("workspace", "tick", "aggregate")
		lit := []*nmstore.Region{
			{Symbol: "CFG/USD", Key: 12, Brightness: 1.08, Counts: 3},
		}

		NewRegions().Publish(tick, "CFG/USD", lit)

		var observed []*nmstore.Region
		tick.Lookup(&observed, "grid/CFG/USD/regions")
		So(len(observed), ShouldEqual, 1)
		So(observed[0].Key, ShouldEqual, uint8(12))

		var train []*nmstore.Region
		tick.Lookup(&train, "train/regions/CFG/USD")
		So(len(train), ShouldEqual, 1)
	})
}
