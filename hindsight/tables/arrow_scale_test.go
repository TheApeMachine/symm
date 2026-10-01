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
	Convey("Standardized/Normalized survive Iceberg arrow provenance", t, func() {
		stdMid, stdSpread := 0.1, 2.5
		normFlow := 0.7
		src := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
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
		})
		src.Label = "BTC/USD"
		src.SeqIdx = 42

		converted, err := arrowSchemaFor(MeasurementSchema())
		So(err, ShouldBeNil)

		recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
		defer recordBuilder.Release()
		recordBuilder.Reserve(1)
		fillMeasurements(recordBuilder, []*data.Measurement[float64]{src}, 1)
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

		grid := store.NewGrid()
		live := src.Clone()
		grid.Update(live)
		grid.Settle()
		liveToken := grid.LitRegions(live)

		grid2 := store.NewGrid()
		grid2.Update(restored[0])
		grid2.Settle()
		So(grid2.LitRegions(restored[0]), ShouldResemble, liveToken)
	})
}
