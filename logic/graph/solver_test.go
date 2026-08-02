package graph

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestExtractMeasurementNodes(t *testing.T) {
	Convey("Given thesis measurements stored by source as slices", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceCVD, []*types.Measurement{{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0).UTC(),
			Metrics: map[string]types.MetricSample{
				"strength": {Raw: 0.75, Unit: types.UnitDimensionless},
				"value":    {Raw: 0.25, Unit: types.UnitDimensionless},
			},
		}})

		solver := NewSolver(nil, nil)
		graph := NewGraph(time.Unix(1, 0).UTC())

		Convey("It should materialize measurement nodes from the slice-backed store", func() {
			solver.extractMeasurementNodes(thesis, graph)
			So(len(graph.Nodes), ShouldEqual, 2)

			strength := graph.Nodes["meas:BTC/USD:cvd:strength"]
			So(strength, ShouldNotBeNil)
			So(strength.Symbol, ShouldEqual, "BTC/USD")
			So(strength.Source, ShouldEqual, "cvd")
			So(strength.Kind, ShouldEqual, Kind("measurement"))
			So(strength.Value, ShouldEqual, 0.75)
		})
	})

	Convey("Given thesis measurements stored as one singleton row", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceCVD, &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "ETH/USD",
			At:     time.Unix(2, 0).UTC(),
			Metrics: map[string]types.MetricSample{
				"strength": {Raw: 0.5, Unit: types.UnitDimensionless},
			},
		})

		solver := NewSolver(nil, nil)
		graph := NewGraph(time.Unix(2, 0).UTC())

		Convey("It should materialize measurement nodes from the singleton fallback", func() {
			solver.extractMeasurementNodes(thesis, graph)
			strength := graph.Nodes["meas:ETH/USD:cvd:strength"]
			So(strength, ShouldNotBeNil)
			So(strength.Symbol, ShouldEqual, "ETH/USD")
			So(strength.Source, ShouldEqual, "cvd")
			So(strength.Kind, ShouldEqual, Kind("measurement"))
			So(strength.Value, ShouldEqual, 0.5)
			So(strength.Metadata["unit"], ShouldEqual, "dimensionless")
		})
	})
}
