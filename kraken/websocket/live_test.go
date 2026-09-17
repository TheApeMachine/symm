package websocket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestLiveStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Live{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestLiveConnections(t *testing.T) {
	Convey("Root activates Level 3 owners explicitly after preparing consumers", t, func() {
		parent := &Live{System: runtime.NewSystem(t.Context(), "private")}
		child := &Live{System: runtime.NewSystem(t.Context(), "level3")}
		child.Transition(runtime.BUSY)
		parent.AttachLevel3("BTC/USD", child)
		So(parent.Connections(), ShouldResemble, []*Live{child})
		So(child.Status(), ShouldEqual, runtime.BUSY)
		parent.Transition(runtime.READY)
		So(child.Status(), ShouldEqual, runtime.BUSY)

		for _, connection := range parent.Connections() {
			connection.Transition(runtime.READY)
		}

		So(child.Status(), ShouldEqual, runtime.READY)
		parent.Transition(runtime.WAITING)
		So(child.Status(), ShouldEqual, runtime.READY)
	})
}
