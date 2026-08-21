package graph

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func newTestMeasurement(id string, source types.SourceType, symbol string, at time.Time) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement(id, string(source), 0, 0)
	measurement.Symbol = symbol
	measurement.At = at

	return measurement
}

func putTestMetric(
	measurement *nmtypes.Measurement,
	metric types.MetricType,
	raw float64,
	normalized *float64,
	unit nmtypes.Unit,
) {
	putTestMetricSide(
		measurement, metric, types.SideNone, raw, normalized, unit,
	)
}

func putTestMetricSide(
	measurement *nmtypes.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
	raw float64,
	normalized *float64,
	unit nmtypes.Unit,
) {
	sample := &nmtypes.Metric[float64]{
		Name: "",
		Raw:  raw,
		Unit: unit,
	}

	if normalized != nil {
		value := *normalized
		sample.Normalized = &value
	}

	measurement.Metrics[types.MetricKey(metric, side)] = sample
}

func TestAddNodes(t *testing.T) {
	Convey("Given timestamped evidence and a separate hypothesis margin", t, func() {
		at := time.Unix(10, 0).UTC()
		observedFrom := at.Add(-time.Second)
		drive := 0.7
		separation := 0.8
		symbol := types.NewSymbol("BTC/USD")
		measurement := newTestMeasurement("cvd-1", types.SourceCVD, "BTC/USD", at)
		measurement.ObservedFrom = observedFrom
		measurement.Horizon = time.Second
		measurement.Maturity = 0.6
		putTestMetric(measurement, types.MetricDrive, drive, &drive, nmtypes.UnitDimensionless)
		putTestMetric(measurement, types.MetricHypothesisSeparation, separation, &separation, nmtypes.UnitDimensionless)
		symbol.AppendMeasurement(measurement)
		graph := NewGraph(at)

		index, err := newMeasurementCompiler().addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), graph,
		)

		Convey("It should retain provenance and keep every emitted metric as evidence", func() {
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
			So(node.Confidence, ShouldAlmostEqual, 0.6*0.8, 1e-12)
			So(node.Maturity, ShouldEqual, 0.6)
			So(node.Metadata["hypothesis_separation"], ShouldEqual, separation)
			So(node.ObservedFrom, ShouldEqual, observedFrom)
			So(node.Horizon, ShouldEqual, time.Second)
			So(index.byReference["cvd:drive"], ShouldHaveLength, 1)
			So(graph.Nodes, ShouldHaveLength, 1)
		})
	})

	Convey("Given a measurement assigned to another pair", t, func() {
		symbol := types.NewSymbol("BTC/USD")
		other := newTestMeasurement("cvd-other", types.SourceCVD, "ETH/USD", time.Unix(11, 0).UTC())
		putTestMetric(other, types.MetricDrive, 0.5, nil, nmtypes.UnitDimensionless)
		symbol.Measurements.Push(other)

		_, err := newMeasurementCompiler().addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			),
			NewGraph(time.Unix(11, 0).UTC()),
		)

		Convey("It should reject cross-pair contamination", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "does not match graph symbol")
		})
	})

	Convey("Given an identified quiet-pass measurement without timestamp or metrics", t, func() {
		symbol := types.NewSymbol("BTC/USD")
		symbol.AppendMeasurement(newTestMeasurement(
			"pumpdump-quiet", types.SourcePumpDump, "BTC/USD", time.Time{},
		))
		graph := NewGraph(time.Unix(12, 0).UTC())

		index, err := newMeasurementCompiler().addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), graph,
		)

		Convey("It should accept the volume-clock row and contribute no nodes", func() {
			So(err, ShouldBeNil)
			So(graph.Nodes, ShouldHaveLength, 0)
			So(index.bySource[string(types.SourcePumpDump)], ShouldHaveLength, 1)
		})
	})

	Convey("Given a metric outside the normalized strength domain", t, func() {
		symbol := types.NewSymbol("BTC/USD")
		measurement := newTestMeasurement(
			"cvd-invalid",
			types.SourceCVD,
			"BTC/USD",
			time.Unix(13, 0).UTC(),
		)
		invalid := 1.01
		putTestMetric(
			measurement,
			types.MetricDrive,
			invalid,
			&invalid,
			nmtypes.UnitDimensionless,
		)
		symbol.AppendMeasurement(measurement)

		Convey("It should fail with the exact producer and value", func() {
			So(func() {
				_, _ = newMeasurementCompiler().addNodes(
					"BTC/USD",
					symbol.MarketMeasurements(
						symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
					),
					NewGraph(time.Unix(13, 0).UTC()),
				)
			}, ShouldPanicWith,
				"graph: normalized measurement strength must be in [0,1]: "+
					"source=cvd symbol=BTC/USD metric=drive value=1.01")
		})
	})
}

func TestAddCategoryEdges(t *testing.T) {
	Convey("Given retained category evidence with old raw and current normalized values", t, func() {
		at := time.Unix(10, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		drive := 0.8
		old := newTestMeasurement("old", types.SourceCVD, "BTC/USD", at)
		putTestMetric(old, types.MetricDrive, 0.7, nil, nmtypes.UnitDimensionless)
		symbol.AppendMeasurement(old)
		current := newTestMeasurement("current", types.SourceCVD, "BTC/USD", at)
		current.Maturity = 1
		putTestMetric(current, types.MetricDrive, drive, &drive, nmtypes.UnitDimensionless)
		putTestMetric(current, types.MetricHypothesisSeparation, 1, nil, nmtypes.UnitDimensionless)
		symbol.AppendMeasurement(current)
		symbol.Categories.Push([]types.Category{{
			Symbol: "BTC/USD", Type: types.CategoryAggressiveDrive,
			Confidence: 0.7,
			Supporting: []string{"cvd:drive"},
		}})
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		solver := NewSolver(types.NewThesis(t.Context(), nil), nil, nil)
		categories := solver.popCategories(symbol)
		solver.extractCategoryNodes(symbol, categories, graph)
		index, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addCategoryEdges(categories, symbol.Symbol, graph, index)

		Convey("It should relate the quality-stamped evidence and skip the unstamped raw node", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"current", "cvd:drive"})
			So(graph.Edges[0].Weight, ShouldAlmostEqual, drive, 1e-12)
			So(graph.Edges[0].Confidence, ShouldEqual, 1)
		})
	})

	Convey("Given category evidence without a normalized measurement value", t, func() {
		at := time.Unix(20, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		raw := newTestMeasurement("cvd-raw", types.SourceCVD, "BTC/USD", at)
		putTestMetric(raw, types.MetricDrive, 0.7, nil, nmtypes.UnitDimensionless)
		symbol.AppendMeasurement(raw)
		symbol.Categories.Push([]types.Category{{
			Symbol: "BTC/USD", Type: types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
		}})
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		categories := NewSolver(
			types.NewThesis(t.Context(), nil), nil, nil,
		).popCategories(symbol)
		index, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addCategoryEdges(categories, symbol.Symbol, graph, index)

		Convey("It should skip the unobservable edge and leave the graph honestly empty", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldBeEmpty)
		})
	})
}

func BenchmarkAddNodes(b *testing.B) {
	at := time.Unix(30, 0).UTC()
	symbol := types.NewSymbol("BTC/USD")
	value := 0.5

	for index, schema := range types.CategorySchemas {
		measurement := newTestMeasurement(
			string(schema.Source)+":"+strconv.Itoa(index),
			schema.Source,
			"BTC/USD",
			at,
		)
		putTestMetric(measurement, schema.Metric, value, &value, nmtypes.UnitDimensionless)
		symbol.AppendMeasurement(measurement)
	}

	compiler := newMeasurementCompiler()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := compiler.addNodes(
			"BTC/USD", symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), NewGraph(at),
		); err != nil {
			b.Fatal(err)
		}
	}
}
