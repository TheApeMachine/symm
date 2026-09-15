package websocket

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"testing"
)

func TestLiveStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Live{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}
