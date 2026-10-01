package cvd

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestGateAdmitsTradeShapedIngress(t *testing.T) {
	Convey("Given price+qty with Provenance side (live trade ingress shape)", t, func() {
		m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
			"price": {Raw: 100.0},
			"qty":   {Raw: 0.5},
		})
		m.SetProvenance("side", "buy")
		m.SetProvenance("channel", "trade")

		gate := NewGate()
		out := data.Read[*data.Measurement[float64]](gate.Next(
			transport.NewOne(unsafe.Pointer(&m)).Next(nil),
		))
		So(out, ShouldNotBeNil)
		So(out.Err, ShouldBeNil)
	})

	Convey("Given volume instead of qty (broken pre-fix ingress)", t, func() {
		m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
			"price":  {Raw: 100.0},
			"volume": {Raw: 0.5},
		})
		m.SetMetadata("side", "buy")

		gate := NewGate()
		out := data.Read[*data.Measurement[float64]](gate.Next(
			transport.NewOne(unsafe.Pointer(&m)).Next(nil),
		))
		So(out, ShouldNotBeNil)
		So(out.Err, ShouldNotBeNil)
	})
}
