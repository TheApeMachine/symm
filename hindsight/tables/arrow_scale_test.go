package tables

import (
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestMeasurementRoundTripRestoresScales(t *testing.T) {
	Convey("Standardized/Normalized/Deformation survive Iceberg arrow provenance", t, func() {
		stdMid, stdSpread := 0.1, 2.5
		normFlow := 0.7
		defSpeed := 0.3333333333333333
		src := data.NewMeasurement("websocket", map[string]data.Metric{
			"mid": {
				Label: "mid", Raw: 100000,
				Standardized: &stdMid,
			},
			"spread": {
				Label: "spread", Raw: 12,
				Standardized: &stdSpread,
			},
			"flow": {
				Label: "flow", Raw: 0.7,
				Normalized: &normFlow,
			},
			"speed": {
				Label: "speed", Raw: 42.0,
				Deformation: &defSpeed,
			},
		})
		src.Label = "BTC/USD"
		src.SeqIdx = 42
		src.Maturity = 1.0

		converted, err := arrowSchemaFor(MeasurementSchema())
		So(err, ShouldBeNil)

		recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
		defer recordBuilder.Release()
		recordBuilder.Reserve(1)
		fillMeasurements(recordBuilder, []*data.Measurement{src}, 1)
		batch := recordBuilder.NewRecord()
		defer batch.Release()

		restored, err := ReadMeasurements(batch)
		So(err, ShouldBeNil)
		So(len(restored), ShouldEqual, 1)

		mid := restored[0].GetMetric("mid")
		So(mid.Standardized, ShouldNotBeNil)
		So(*mid.Standardized, ShouldEqual, stdMid)
		spread := restored[0].GetMetric("spread")
		So(spread.Standardized, ShouldNotBeNil)
		So(*spread.Standardized, ShouldEqual, stdSpread)
		flow := restored[0].GetMetric("flow")
		So(flow.Normalized, ShouldNotBeNil)
		So(*flow.Normalized, ShouldEqual, normFlow)
		speed := restored[0].GetMetric("speed")
		So(speed.Deformation, ShouldNotBeNil)
		So(*speed.Deformation, ShouldEqual, defSpeed)

		grid := store.NewGrid()
		grid.Update(src)
		grid.Settle()
		liveToken := grid.LitRegions(src)

		grid2 := store.NewGrid()
		grid2.Update(restored[0])
		grid2.Settle()
		So(grid2.LitRegions(restored[0]), ShouldResemble, liveToken)
	})
}
