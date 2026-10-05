package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMeasurementToWirePeerDepthOne(t *testing.T) {
	Convey("Peers encode one level; nested peer.Peers stay off the wire", t, func() {
		grandchild := data.NewMeasurement("nested", map[string]data.Metric{
			"deep": {Raw: 99, Label: "deep"},
		})
		child := data.NewMeasurement("hawkes:trade", map[string]data.Metric{
			"arrival_rate": {Raw: 1, Label: "arrival_rate"},
		})
		child.Peers = []*data.Measurement{grandchild}

		root := data.NewMeasurement("websocket", map[string]data.Metric{
			"price": {Raw: 10, Label: "price"},
		})
		root.Peers = []*data.Measurement{child}

		alloc := data.NewAllocator()
		defer data.Free(alloc)

		wire := MeasurementToWire(root, alloc, true)
		So(wire, ShouldNotBeNil)
		So(len(wire.Peers), ShouldEqual, 1)
		So(wire.Peers[0].Source, ShouldEqual, "hawkes:trade")
		So(len(wire.Peers[0].Peers), ShouldEqual, 0)
	})
}
