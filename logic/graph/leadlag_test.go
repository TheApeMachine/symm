package graph

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestAddLeadLagEdges(t *testing.T) {
	Convey("Given supported price relationships against a measured peer", t, func() {
		at := time.Unix(10, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		local := types.NewSymbol("ALT/USD", nil)
		peer := types.NewSymbol("BTC/USD", nil)
		localMeasurement := leadLagFixture(
			"local-leadlag", "ALT/USD", "BTC/USD", at, 1, 0.6, 0.2, 0.1, 1,
		)
		peerMeasurement := leadLagFixture(
			"peer-leadlag", "BTC/USD", "", at, 1, 0, 0, 0, 0,
		)
		local.Measurements = append(local.Measurements, localMeasurement)
		peer.Measurements = append(peer.Measurements, peerMeasurement)
		thesis.Symbols.Store("ALT/USD", local)
		thesis.Symbols.Store("BTC/USD", peer)
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(local, graph)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(thesis, local, graph, index)

		Convey("It should preserve temporal, synchronized, and decoupled evidence", func() {
			So(err, ShouldBeNil)
			counts := make(map[RelationType]int)

			for _, edge := range graph.Edges {
				counts[edge.Relation]++
				So(edge.Evidence[0], ShouldEqual, localMeasurement.ID)
				So(*edge.Quality, ShouldEqual, 0.8)
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
		thesis := types.NewThesis(t.Context(), nil)
		local := types.NewSymbol("ALT/USD", nil)
		peer := types.NewSymbol("BTC/USD", nil)
		local.Measurements = append(local.Measurements, leadLagFixture(
			"local-empty", "ALT/USD", "BTC/USD", at, 0, 0, 0, 0, 0,
		))
		peer.Measurements = append(peer.Measurements, leadLagFixture(
			"peer-empty", "BTC/USD", "", at, 0, 0, 0, 0, 0,
		))
		thesis.Symbols.Store("ALT/USD", local)
		thesis.Symbols.Store("BTC/USD", peer)
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(local, graph)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(thesis, local, graph, index)

		Convey("It should state that the price paths are incomparable", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 2)
			So(graph.Edges[0].Relation, ShouldEqual, RelationIncomparableWith)
			So(graph.Edges[1].Relation, ShouldEqual, RelationIncomparableWith)
			So(graph.Edges[0].Weight, ShouldEqual, 1.0)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"local-empty", "leadlag:sample_support"})
		})
	})

	Convey("Given price observations separated beyond the older evidence horizon", t, func() {
		at := time.Unix(30, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		local := types.NewSymbol("ALT/USD", nil)
		peer := types.NewSymbol("BTC/USD", nil)
		localMeasurement := leadLagFixture(
			"local-stale", "ALT/USD", "BTC/USD", at, 1, 0, 0, 0, 0,
		)
		peerMeasurement := leadLagFixture(
			"peer-stale", "BTC/USD", "", at.Add(-3*time.Second), 1, 0, 0, 0, 0,
		)
		peerMeasurement.Horizon = time.Second
		local.Measurements = append(local.Measurements, localMeasurement)
		peer.Measurements = append(peer.Measurements, peerMeasurement)
		thesis.Symbols.Store("ALT/USD", local)
		thesis.Symbols.Store("BTC/USD", peer)
		graph := NewGraph(at)
		compiler := newMeasurementCompiler()
		index, err := compiler.addNodes(local, graph)
		So(err, ShouldBeNil)

		err = compiler.addLeadLagEdges(thesis, local, graph, index)

		Convey("It should state the exact horizon-relative staleness", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Relation, ShouldEqual, RelationStaleRelativeTo)
			So(graph.Edges[0].From, ShouldEqual,
				measurementNodeID(
					peerMeasurement,
					types.MetricKey(types.MetricLastPrice, types.SideNone),
				),
			)
			So(graph.Edges[0].Weight, ShouldEqual, 3.0)
			So(graph.Edges[0].Horizon, ShouldEqual, time.Second)
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
) *types.Measurement {
	price := 100.0
	quality := 0.8
	zero := 0.0

	return &types.Measurement{
		ID: id, Source: types.SourceLeadLag, Symbol: symbol, Peer: peer, At: at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricLastPrice, types.SideNone): {
				Raw: price, Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricSampleSupport, types.SideNone): {
				Raw: support, Normalized: &support, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricInefficient, types.SideNone): {
				Raw: inefficient, Normalized: &inefficient, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSync, types.SideNone): {
				Raw: synchronized, Normalized: &synchronized, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricDecoupled, types.SideNone): {
				Raw: decoupled, Normalized: &decoupled, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedLagDirection, types.SideNone): {
				Raw: direction, Normalized: &direction, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedContempCorrelation, types.SideNone): {
				Raw: zero, Normalized: &zero, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedLagCorrelation, types.SideNone): {
				Raw: zero, Normalized: &zero, Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSNR, types.SideNone): {
				Raw: quality, Normalized: &quality, Unit: types.UnitDimensionless,
			},
		},
	}
}
