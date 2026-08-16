package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	logicgraph "github.com/theapemachine/symm/logic/graph"
)

func TestGraphPerspective(t *testing.T) {
	Convey("Given support, contradiction, and context for an explicit opportunity", t, func() {
		graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
		graph.DecisionTarget = "hyp:BTC/USD:long_opportunity"
		graph.AddNode(&logicgraph.Node{ID: graph.DecisionTarget, Kind: logicgraph.KindHypothesis})
		graph.AddNode(&logicgraph.Node{ID: "support", Kind: logicgraph.KindCategory})
		graph.AddNode(&logicgraph.Node{ID: "oppose", Kind: logicgraph.KindCategory})
		graph.AddNode(&logicgraph.Node{ID: "context", Kind: logicgraph.KindManifold})
		graph.AddEdge(&logicgraph.Edge{
			From: "support", To: graph.DecisionTarget,
			Relation: logicgraph.RelationSupports, Weight: 0.8, Confidence: 0.75,
		})
		graph.AddEdge(&logicgraph.Edge{
			From: "oppose", To: graph.DecisionTarget,
			Relation: logicgraph.RelationContradicts, Weight: 0.2, Confidence: 0.5,
		})
		graph.AddEdge(&logicgraph.Edge{
			From: "context", To: graph.DecisionTarget,
			Relation: logicgraph.RelationConditions, Weight: 1, Confidence: 0.4,
		})

		perspective, err := graphPerspective(graph)

		Convey("It should preserve semantics without manufacturing a return", func() {
			So(err, ShouldBeNil)
			So(perspective.Hypothesis, ShouldEqual, graph.DecisionTarget)
			So(perspective.Support, ShouldAlmostEqual, 0.6)
			So(perspective.Contradiction, ShouldAlmostEqual, 0.1)
			So(perspective.Conditions, ShouldAlmostEqual, 0.4)
			So(perspective.Balance, ShouldAlmostEqual, 5.0/7.0)
			So(perspective.Confidence, ShouldAlmostEqual, 0.7)
			So(perspective.Score, ShouldAlmostEqual, 0.5)
			So(perspective.Direction, ShouldEqual, 1)
		})
	})

	Convey("Given context without directional evidence", t, func() {
		graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
		graph.DecisionTarget = "hyp:BTC/USD:long_opportunity"
		graph.AddNode(&logicgraph.Node{ID: graph.DecisionTarget, Kind: logicgraph.KindHypothesis})

		_, err := graphPerspective(graph)
		So(err, ShouldNotBeNil)
	})
}
