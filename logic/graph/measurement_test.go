package graph

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestAddNodes(t *testing.T) {
	Convey("Given timestamped evidence and a separate hypothesis margin", t, func() {
		at := time.Unix(10, 0).UTC()
		observedFrom := at.Add(-time.Second)
		drive := 0.7
		separation := 0.8
		symbol := types.NewSymbol("BTC/USD", nil)
		measurement := &types.Measurement{
			ID: "cvd-1", Source: types.SourceCVD, Symbol: "BTC/USD", At: at,
			ObservedFrom: observedFrom, Horizon: time.Second, Maturity: 0.6,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Raw: drive, Normalized: &drive, Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricHypothesisSeparation, types.SideNone): {
					Raw: separation, Normalized: &separation, Unit: types.UnitDimensionless,
				},
			},
		}
		symbol.AppendMeasurements([]*types.Measurement{measurement})
		graph := NewGraph(at)

		index, err := newMeasurementCompiler().addNodes(
			"BTC/USD", symbol.MarketMeasurements("graph"), graph,
		)

		Convey("It should retain provenance without presenting separation as quality", func() {
			So(err, ShouldBeNil)
			So(graph.Nodes, ShouldHaveLength, 1)
			node := graph.Nodes[measurementNodeID(measurement, "drive")]
			So(node, ShouldNotBeNil)
			So(node.ID, ShouldEqual, "meas:BTC/USD:cvd:drive")
			So(node.Kind, ShouldEqual, KindMeasurement)
			So(node.MeasurementID, ShouldEqual, "cvd-1")
			So(node.Metric, ShouldEqual, types.MetricDrive)
			So(node.Value, ShouldEqual, drive)
			So(*node.Normalized, ShouldEqual, drive)
			So(node.Quality, ShouldBeNil)
			So(node.Maturity, ShouldEqual, 0.6)
			So(node.ObservedFrom, ShouldEqual, observedFrom)
			So(node.Horizon, ShouldEqual, time.Second)
			So(index.byReference["cvd:drive"], ShouldHaveLength, 1)
		})
	})

	Convey("Given a measurement assigned to another pair", t, func() {
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
			ID: "cvd-other", Source: types.SourceCVD, Symbol: "ETH/USD",
			At: time.Unix(11, 0).UTC(),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {Raw: 0.5},
			},
		}})

		_, err := newMeasurementCompiler().addNodes(
			"BTC/USD", symbol.MarketMeasurements("graph"),
			NewGraph(time.Unix(11, 0).UTC()),
		)

		Convey("It should reject cross-pair contamination", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "does not match graph symbol")
		})
	})
}

func TestAddCategoryEdges(t *testing.T) {
	Convey("Given retained category evidence with old raw and current normalized values", t, func() {
		at := time.Unix(10, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		drive := 0.8
		symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
			ID: "old", Source: types.SourceCVD, Symbol: "BTC/USD", At: at,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Raw: 0.7, Unit: types.UnitDimensionless,
				},
			},
		}})
		symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
			ID: "current", Source: types.SourceCVD, Symbol: "BTC/USD", At: at,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Raw: drive, Normalized: &drive, Unit: types.UnitDimensionless,
				},
			},
		}})
		symbol.Categories.Store("BTC/USD", []types.Category{{
			Symbol: "BTC/USD", Type: types.CategoryAggressiveDrive,
			Confidence: 0.7,
			Supporting: []string{"cvd:drive"},
		}})
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		NewSolver(nil, nil).extractCategoryNodes(symbol, graph)
		index, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addCategoryEdges(symbol, graph, index)

		Convey("It should relate the usable evidence and skip the raw stale node", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"current", "cvd:drive"})
			So(graph.Edges[0].Confidence, ShouldEqual, 0.7)
		})
	})

	Convey("Given category evidence without a normalized measurement value", t, func() {
		at := time.Unix(20, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
			ID: "cvd-raw", Source: types.SourceCVD, Symbol: "BTC/USD", At: at,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Raw: 0.7, Unit: types.UnitDimensionless,
				},
			},
		}})
		symbol.Categories.Store("BTC/USD", []types.Category{{
			Symbol: "BTC/USD", Type: types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
		}})
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addCategoryEdges(symbol, graph, index)

		Convey("It should reject the unsupported edge instead of substituting a value", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "has no measurement node")
			So(graph.Edges, ShouldBeEmpty)
		})
	})
}

func BenchmarkAddNodes(b *testing.B) {
	at := time.Unix(30, 0).UTC()
	symbol := types.NewSymbol("BTC/USD", nil)
	value := 0.5

	for index, schema := range types.CategorySchemas {
		symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
			ID:     string(schema.Source) + ":" + strconv.Itoa(index),
			Source: schema.Source, Symbol: "BTC/USD", At: at,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(schema.Metric, schema.Side): {
					Raw: value, Normalized: &value, Unit: types.UnitDimensionless,
				},
			},
		}})
	}

	compiler := newMeasurementCompiler()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements("graph"), NewGraph(at),
		); err != nil {
			b.Fatal(err)
		}
	}
}
