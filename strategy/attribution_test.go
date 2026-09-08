package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestAttributionReport(t *testing.T) {
	Convey("Encoded directional conditions retain their source and metric labels", t, func() {
		store := attribution{}
		token := learning.ConditionToken(2, -1, 1)
		So(store.observe([]uint64{token}, types.ActionScale, -0.03, 1), ShouldBeNil)
		report := store.report([][2]string{{"leadlag", "lag_fraction"}, {"resonance", "readout.148"}})
		So(report, ShouldHaveLength, 1)
		So(report[0].Source, ShouldEqual, "resonance")
		So(report[0].Label, ShouldEqual, "readout.148")
		So(report[0].Prior.Mean, ShouldBeLessThan, 0)
	})
}
