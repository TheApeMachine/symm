package graph

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestExtractMeasurementNodes(t *testing.T) {
	Convey("Given thesis measurements stored by source as slices", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements.Store(types.SourceCVD, []*types.Measurement{{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0).UTC(),
			Metrics: map[string]types.MetricSample{
				"strength": {Raw: 0.75, Unit: types.UnitDimensionless},
				"value":    {Raw: 0.25, Unit: types.UnitDimensionless},
			},
		}})

		solver := NewSolver(nil)
		graph := NewGraph(time.Unix(1, 0).UTC())

		Convey("It should materialize measurement nodes from the slice-backed store", func() {
			solver.extractMeasurementNodes(thesis, graph)
			So(len(graph.Nodes), ShouldEqual, 2)

			strength := graph.Nodes["meas:BTC/USD:cvd:strength"]
			So(strength, ShouldNotBeNil)
			So(strength.Symbol, ShouldEqual, "BTC/USD")
			So(strength.Source, ShouldEqual, "cvd")
			So(strength.Kind, ShouldEqual, "measurement")
			So(strength.Value, ShouldEqual, 0.75)
		})
	})
}
