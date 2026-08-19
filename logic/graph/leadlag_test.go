package graph

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestAddLeadLagEdges(t *testing.T) {
	Convey("Given supported price relationships against a measured peer", t, func() {
		at := time.Unix(10, 0).UTC()
		local := types.NewSymbol("ALT/USD", nil)
		localMeasurement := leadLagFixture(
			"local-leadlag", "ALT/USD", "BTC/USD", at, 1, 0.6, 0.2, 0.1, 1,
		)
		local.AppendMeasurement(localMeasurement)
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(
			"ALT/USD", local.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(local, graph, index)

		Convey("It should preserve temporal, synchronized, and decoupled evidence", func() {
			So(err, ShouldBeNil)
			peerPrice := graph.Nodes[measurementNodeID(
				localMeasurement,
				types.MetricKey(types.MetricPeerLastPrice, types.SideNone),
			)]
			So(peerPrice.Symbol, ShouldEqual, "BTC/USD")
			So(peerPrice.Value, ShouldEqual, 200.0)
			So(peerPrice.At, ShouldEqual, at)
			counts := make(map[RelationType]int)

			for _, edge := range graph.Edges {
				counts[edge.Relation]++
				So(edge.Evidence[0], ShouldEqual, localMeasurement.ID)
				So(edge.Quality, ShouldBeNil)
			}

			So(counts[RelationLeads], ShouldEqual, 1)
			So(counts[RelationLags], ShouldEqual, 1)
			So(counts[RelationRedundantWith], ShouldEqual, 2)
			So(counts[RelationIndependentOf], ShouldEqual, 2)
			So(graph.Edges[0].Evidence, ShouldContain,
				"leadlag:inefficient")
			So(graph.Edges[0].Evidence, ShouldContain,
				"leadlag:signed_lag_direction")
		})
	})

	Convey("Given a peer comparison without one resolved return", t, func() {
		at := time.Unix(20, 0).UTC()
		local := types.NewSymbol("ALT/USD", nil)
		local.AppendMeasurement(leadLagFixture(
			"local-empty", "ALT/USD", "BTC/USD", at, 0, 0, 0, 0, 0,
		))
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(
			"ALT/USD", local.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(local, graph, index)

		Convey("It should state that the price paths are incomparable", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 2)
			So(graph.Edges[0].Relation, ShouldEqual, RelationIncomparableWith)
			So(graph.Edges[1].Relation, ShouldEqual, RelationIncomparableWith)
			So(graph.Edges[0].Weight, ShouldEqual, 1.0)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"local-empty", "leadlag:sample_count"})
		})
	})

	Convey("Given price observations separated beyond the older evidence horizon", t, func() {
		at := time.Unix(30, 0).UTC()
		local := types.NewSymbol("ALT/USD", nil)
		localMeasurement := leadLagFixture(
			"local-stale", "ALT/USD", "BTC/USD", at, 1, 0, 0, 0, 0,
		)
		localMeasurement.PeerAt = at.Add(-3 * time.Second)
		localMeasurement.PeerObservedFrom = localMeasurement.PeerAt.Add(-time.Second)
		local.AppendMeasurement(localMeasurement)
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(
			"ALT/USD", local.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(local, graph, index)

		Convey("It should state the exact horizon-relative staleness", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Relation, ShouldEqual, RelationStaleRelativeTo)
			So(graph.Edges[0].From, ShouldEqual,
				measurementNodeID(
					localMeasurement,
					types.MetricKey(types.MetricPeerLastPrice, types.SideNone),
				),
			)
			So(graph.Edges[0].Weight, ShouldEqual, 3.0)
			So(graph.Edges[0].Horizon, ShouldEqual, time.Second)
		})
	})

	Convey("Given retained lead-lag relationships from different anchor epochs", t, func() {
		at := time.Unix(40, 0).UTC()
		local := types.NewSymbol("ALT/USD", nil)
		local.AppendMeasurement(leadLagFixture(
			"old", "ALT/USD", "UNFI/USD", at.Add(-time.Second), 0, 0, 0, 0, 0,
		))
		local.AppendMeasurement(leadLagFixture(
			"current", "ALT/USD", "SOSO/USD", at, 0, 0, 0, 0, 0,
		))
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(
			"ALT/USD", local.MarketMeasurements("graph"), graph,
		)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(local, graph, index)

		Convey("It should relate every queued anchor epoch", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 4)

			seen := map[string]bool{}

			for _, edge := range graph.Edges {
				seen[edge.Evidence[0]] = true
			}

			So(seen["old"], ShouldBeTrue)
			So(seen["current"], ShouldBeTrue)
		})
	})
}

func leadLagFixture(
	id string,
	symbol string,
	peer string,
	at time.Time,
	support float64,
	inefficient float64,
	synchronized float64,
	decoupled float64,
	direction float64,
) *nmtypes.Measurement {
	price := 100.0
	peerPrice := 200.0
	quality := 0.8
	zero := 0.0

	measurement := nmtypes.NewMeasurement(id, string(types.SourceLeadLag), 0, 0)
	measurement.Symbol = symbol
	measurement.Peer = peer
	measurement.At = at
	measurement.Metrics = map[string]*nmtypes.Metric[float64]{
		types.MetricKey(types.MetricLastPrice, types.SideNone): nmtypes.NewMetric(
			"", price, nmtypes.Descriptor{Unit: nmtypes.UnitQuoteCurrency},
		),
		types.MetricKey(types.MetricSampleCount, types.SideNone): nmtypes.NewMetric(
			"", support, nmtypes.Descriptor{Unit: nmtypes.UnitCount},
		),
		types.MetricKey(types.MetricInefficient, types.SideNone): nmtypes.NewNormalizedMetric(
			"", inefficient, inefficient, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricSync, types.SideNone): nmtypes.NewNormalizedMetric(
			"", synchronized, synchronized, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricDecoupled, types.SideNone): nmtypes.NewNormalizedMetric(
			"", decoupled, decoupled, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricSignedLagDirection, types.SideNone): nmtypes.NewNormalizedMetric(
			"", direction, direction, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricSignedContempCorrelation, types.SideNone): nmtypes.NewNormalizedMetric(
			"", zero, zero, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricSignedLagCorrelation, types.SideNone): nmtypes.NewNormalizedMetric(
			"", zero, zero, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
		types.MetricKey(types.MetricHypothesisSeparation, types.SideNone): nmtypes.NewNormalizedMetric(
			"", quality, quality, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless},
		),
	}

	if peer != "" {
		measurement.PeerAt = at
		measurement.PeerObservedFrom = at.Add(-time.Second)
		measurement.Metrics[types.MetricKey(
			types.MetricPeerLastPrice,
			types.SideNone,
		)] = nmtypes.NewMetric("", peerPrice, nmtypes.Descriptor{Unit: nmtypes.UnitQuoteCurrency})
	}

	return measurement
}
