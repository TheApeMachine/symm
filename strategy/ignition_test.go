package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestIgnitionClears(t *testing.T) {
	Convey("ignition refuses flat and sub-floor clocks", t, func() {
		ignition := NewIgnition()
		floor := system.Cfg.Learning.GateIgnitionFloor

		So(ignition.Clears(0), ShouldBeFalse)
		So(ignition.Clears(-0.1), ShouldBeFalse)

		if floor > 0 {
			So(ignition.Clears(floor/2), ShouldBeFalse)
		}

		So(ignition.Clears(floor), ShouldBeTrue)
		So(ignition.Clears(floor+0.1), ShouldBeTrue)
	})

	Convey("nil ignition never clears", t, func() {
		var ignition *Ignition
		So(ignition.Clears(1.0), ShouldBeFalse)
	})
}
