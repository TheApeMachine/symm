package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMeasurementToWirePeerDepthOne(t *testing.T) {
	Convey("Peers encode one level; nested peer.Peers stay off the wire", t, func() {
		grandchild := data.NewMeasurement("nested", map[string]data.Metric[float64]{
			"deep": {Raw: 99, Label: "deep"},
		})
		child := data.NewMeasurement("hawkes:trade", map[string]data.Metric[float64]{
			"arrival_rate": {Raw: 1, Label: "arrival_rate"},
		})
		child.Peers = []*data.Measurement[float64]{grandchild}

		root := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
			"price": {Raw: 10, Label: "price"},
		})
		root.Peers = []*data.Measurement[float64]{child}

		alloc := data.NewAllocator()
		defer data.Free(alloc)

		wire := MeasurementToWire(root, alloc)
		So(wire, ShouldNotBeNil)
		So(len(wire.Peers), ShouldEqual, 1)
		So(wire.Peers[0].Source, ShouldEqual, "hawkes:trade")
		So(len(wire.Peers[0].Peers), ShouldEqual, 0)
	})
}
