package strategy

import (
	"math"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	logicgraph "github.com/theapemachine/symm/logic/graph"
)

func TestNewGraphEvidence(t *testing.T) {
	Convey("Given direct and unrelated market-graph relations", t, func() {
		marketGraph := graphEvidenceFixture()
		forecastID := "res:BTC/USD:forecast"

		for _, nodeID := range []string{
			"causal:BTC/USD:support",
			"causal:BTC/USD:contradiction",
			"causal:BTC/USD:condition",
		} {
			marketGraph.AddNode(&logicgraph.Node{
				ID:     nodeID,
				Symbol: "BTC/USD",
				Kind:   logicgraph.KindCausal,
				At:     marketGraph.At,
			})
		}

		marketGraph.AddEdge(&logicgraph.Edge{
			From: forecastID, To: "causal:BTC/USD:support",
			Relation: logicgraph.RelationSupports, Weight: 1,
			Confidence: 0.8, At: marketGraph.At,
		})
		marketGraph.AddEdge(&logicgraph.Edge{
			From: forecastID, To: "causal:BTC/USD:contradiction",
			Relation: logicgraph.RelationContradicts, Weight: 0.5,
			Confidence: 0.4, At: marketGraph.At,
		})
		marketGraph.AddEdge(&logicgraph.Edge{
			From: forecastID, To: "causal:BTC/USD:condition",
			Relation: logicgraph.RelationConditions, Weight: 1,
			Confidence: 1, At: marketGraph.At,
		})

		evidence, err := newGraphEvidence(marketGraph, "BTC/USD")

		Convey("It should average only direct directional evidence", func() {
			So(err, ShouldBeNil)
			So(evidence.relations, ShouldEqual, 2)
			So(evidence.supports, ShouldAlmostEqual, 0.4)
			So(evidence.contradicts, ShouldAlmostEqual, 0.1)
		})
	})

	Convey("Given a malformed direct relation", t, func() {
		marketGraph := graphEvidenceFixture()
		marketGraph.AddNode(&logicgraph.Node{
			ID: "causal:BTC/USD:uplift", Symbol: "BTC/USD",
			Kind: logicgraph.KindCausal, At: marketGraph.At,
		})
		marketGraph.AddEdge(&logicgraph.Edge{
			From: "res:BTC/USD:forecast", To: "causal:BTC/USD:uplift",
			Relation: logicgraph.RelationSupports, Weight: math.Inf(1),
			Confidence: 1, At: marketGraph.At,
		})

		_, err := newGraphEvidence(marketGraph, "BTC/USD")

		Convey("It should reject evidence outside the confidence domain", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGraphAdjustedForecast(t *testing.T) {
	Convey("Given a graph forecast that differs from its Thesis forecast", t, func() {
		thesis := plannerThesisFixture(t, "BTC/USD", 0.8)
		stored, _ := thesis.Graphs.Load(marketGraphKey)
		marketGraph := stored.(*logicgraph.Graph)
		marketGraph.Nodes["res:BTC/USD:forecast"].Value++

		_, _, _, err := graphAdjustedForecast(thesis, "BTC/USD")

		Convey("It should reject evidence compiled from another forecast", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGraphEvidenceConfidence(t *testing.T) {
	Convey("Given bounded graph evidence", t, func() {
		Convey("Support should convert the matching share of uncertainty", func() {
			confidence, err := (graphEvidence{supports: 0.5}).Confidence(0.6)

			So(err, ShouldBeNil)
			So(confidence, ShouldAlmostEqual, 0.75)
		})

		Convey("Contradiction should remove the matching share of confidence", func() {
			confidence, err := (graphEvidence{contradicts: 0.5}).Confidence(0.9)

			So(err, ShouldBeNil)
			So(confidence, ShouldAlmostEqual, 0.8181818181818181)
		})

		Convey("Absent directional evidence should remain neutral", func() {
			confidence, err := (graphEvidence{}).Confidence(0.75)

			So(err, ShouldBeNil)
			So(confidence, ShouldEqual, 0.75)
		})
	})
}

func TestGraphEvidenceReward(t *testing.T) {
	Convey("Given graph evidence and realized causal targets", t, func() {
		rows := [][]float64{{0, 0, 0, -2}, {0, 0, 1, 2}}

		Convey("It should express signed evidence on the target RMS scale", func() {
			reward, err := (graphEvidence{
				supports: 0.4, contradicts: 0.1,
			}).Reward(rows, 3)

			So(err, ShouldBeNil)
			So(reward, ShouldAlmostEqual, 0.6)
		})

		Convey("It should reject malformed target history", func() {
			_, err := (graphEvidence{}).Reward([][]float64{{0}}, 3)

			So(err, ShouldNotBeNil)
		})
	})
}

func graphEvidenceFixture() *logicgraph.Graph {
	at := time.Unix(1, 0).UTC()
	marketGraph := logicgraph.NewGraph(at)
	marketGraph.AddNode(&logicgraph.Node{
		ID: "res:BTC/USD:forecast", Symbol: "BTC/USD",
		Kind: logicgraph.KindResonance, At: at,
	})

	return marketGraph
}

func BenchmarkNewGraphEvidence(b *testing.B) {
	marketGraph := graphEvidenceFixture()
	forecastID := "res:BTC/USD:forecast"

	for index := range 256 {
		nodeID := "causal:BTC/USD:" + strconv.Itoa(index)
		marketGraph.AddNode(&logicgraph.Node{
			ID: nodeID, Symbol: "BTC/USD",
			Kind: logicgraph.KindCausal, At: marketGraph.At,
		})
		marketGraph.AddEdge(&logicgraph.Edge{
			From: forecastID, To: nodeID,
			Relation: logicgraph.RelationSupports, Weight: 0.5,
			Confidence: 0.8, At: marketGraph.At,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := newGraphEvidence(marketGraph, "BTC/USD")

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphEvidenceReward(b *testing.B) {
	rows := make([][]float64, 256)

	for index := range rows {
		rows[index] = []float64{0, 0, float64(index % 2), math.Sin(float64(index))}
	}

	evidence := graphEvidence{supports: 0.4, contradicts: 0.1}
	b.ReportAllocs()

	for b.Loop() {
		_, err := evidence.Reward(rows, 3)

		if err != nil {
			b.Fatal(err)
		}
	}
}
