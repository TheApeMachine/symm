package graph

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
The query-focused Influence Graph tests live here, mirrored against the
methods under test: Incoming, Outgoing, Relation, History, FamilyEdges, and
Paths. Registration, mutation, serialization, and validation tests stay in
influence_test.go.
*/

func TestIncoming(t *testing.T) {
	Convey("Given an influence edge into a target", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")
		edge := testEdge(EdgeInfluence, source, target, 0, time.Second, 0.01)
		So(influenceGraph.UpsertEdge(edge), ShouldBeNil)

		Convey("incoming queries retain the relation statistics", func() {
			incoming := influenceGraph.Incoming(target)
			So(len(incoming), ShouldEqual, 1)
			So(incoming[0].Result, ShouldEqual, edge.Result)
		})
	})
}

func TestOutgoing(t *testing.T) {
	Convey("Given an influence edge from a source", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")
		edge := testEdge(EdgeInfluence, source, target, 0.2, time.Second, 0.1)
		So(influenceGraph.UpsertEdge(edge), ShouldBeNil)

		Convey("outgoing queries retain the relation statistics", func() {
			outgoing := influenceGraph.Outgoing(source)
			So(len(outgoing), ShouldEqual, 1)
			So(outgoing[0].Result, ShouldEqual, edge.Result)
		})
	})
}

func TestRelation(t *testing.T) {
	Convey("Given an influence graph with a relation", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")

		Convey("the current edge is returned with full statistics", func() {
			edge := testEdge(EdgeInfluence, source, target, 0.4, 2*time.Second, 0.3)
			So(influenceGraph.UpsertEdge(edge), ShouldBeNil)

			current := influenceGraph.Relation(source, target)
			So(current, ShouldNotBeNil)
			So(current.Result.Lag, ShouldEqual, 2*time.Second)
			So(*current.Result.Coefficient, ShouldEqual, 0.3)
		})

		Convey("a missing relation returns nil", func() {
			So(influenceGraph.Relation(source, target), ShouldBeNil)
		})
	})
}

func TestHistory(t *testing.T) {
	Convey("Given an edge with multiple measurements over time", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")

		first := testEdge(EdgeInfluence, source, target, 0.1, time.Second, 0.2)
		first.At = time.Unix(100, 0)
		second := testEdge(EdgeInfluence, source, target, 0.5, 2*time.Second, -0.3)
		second.At = time.Unix(200, 0)

		So(influenceGraph.UpsertEdge(first), ShouldBeNil)
		So(influenceGraph.UpsertEdge(second), ShouldBeNil)

		Convey("history is retained in chronological order", func() {
			history := influenceGraph.History(source, target)
			So(len(history), ShouldEqual, 2)
			So(history[0].At, ShouldEqual, first.At)
			So(history[1].At, ShouldEqual, second.At)
		})

		Convey("current values do not erase historical edge state", func() {
			current := influenceGraph.Relation(source, target)
			So(current.Result.Lag, ShouldEqual, 2*time.Second)
			So(*current.Result.Coefficient, ShouldEqual, -0.3)
			So(influenceGraph.History(source, target), ShouldHaveLength, 2)
		})
	})
}

func TestFamilyEdges(t *testing.T) {
	Convey("Given edges from different sources into one target", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")
		other := testCoordinate("hawkes", "conditional_intensity")

		So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, source, target, 0.3, time.Second, 0.2)), ShouldBeNil)
		So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, other, target, 0.4, time.Second, 0.1)), ShouldBeNil)

		Convey("family rollups expose the underlying coordinate edges", func() {
			rollup := influenceGraph.FamilyEdges(
				relation.Selector{Source: "cvd"},
				relation.Selector{Metric: "midpoint_log_return"},
			)
			So(len(rollup), ShouldEqual, 1)
			So(rollup[0].Source, ShouldEqual, source)

			allTowards := influenceGraph.FamilyEdges(
				relation.Selector{},
				relation.Selector{Metric: "midpoint_log_return"},
			)
			So(len(allTowards), ShouldEqual, 2)
		})
	})
}

func TestPaths(t *testing.T) {
	Convey("Given a mediated chain of influence edges", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")
		mediator := testCoordinate("cvd", "net_notional_rate")

		So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, source, mediator, 0.5, time.Second, 0.8)), ShouldBeNil)
		So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, mediator, target, 0.6, time.Second, 0.9)), ShouldBeNil)

		Convey("paths between coordinates retain edge measurements", func() {
			paths := influenceGraph.Paths(source, target, 4)
			So(len(paths), ShouldEqual, 1)
			So(len(paths[0]), ShouldEqual, 2)
			So(paths[0][0].Result.Lag, ShouldEqual, time.Second)
		})
	})
}
