package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	logicgraph "github.com/theapemachine/symm/logic/graph"
)

func TestDecisionPerspective(t *testing.T) {
	Convey("Given predictive, causal, and manifold return estimates", t, func() {
		graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
		graph.Forecast = &learning.RLSOutput{Value: 0.01, Ready: true}
		graph.ForecastHorizon = 4
		graph.AddNode(&logicgraph.Node{
			ID: "causal:BTC/USD:doExpectation", Kind: logicgraph.KindCausal,
			Value: -0.02, Confidence: 0.5, Metadata: map[string]any{"horizon": 1},
		})
		graph.AddNode(&logicgraph.Node{
			ID: "man:BTC/USD:phase_alignment", Kind: logicgraph.KindManifold,
			Value: 0.03, Confidence: 1, Metadata: map[string]any{"horizon": 8},
		})

		value, confidence, sources, err := decisionPerspective(graph, 0.9)

		Convey("It should combine only priced returns, never the direction lean", func() {
			So(err, ShouldBeNil)
			So(sources, ShouldHaveLength, 2)
			So(sources[0].Source, ShouldEqual, "causal")
			So(sources[1].Source, ShouldEqual, "manifold")
			So(value, ShouldAlmostEqual, (-0.08*0.5+0.015)/(0.5+1), 1e-12)
			So(confidence, ShouldAlmostEqual, (0.5+1)/2, 1e-12)
			So(sources[0].Horizon, ShouldEqual, 4)
			So(sources[1].Horizon, ShouldEqual, 4)
		})

		Convey("A materially different causal estimate should change the perspective", func() {
			graph.Nodes["causal:BTC/USD:doExpectation"].Value = -0.2
			changed, _, _, changedErr := decisionPerspective(graph, 0.9)
			So(changedErr, ShouldBeNil)
			So(changed, ShouldBeLessThan, value)
		})

		Convey("A return estimate without its native horizon should be rejected", func() {
			delete(graph.Nodes["causal:BTC/USD:doExpectation"].Metadata, "horizon")
			_, _, _, missingHorizonErr := decisionPerspective(graph, 0.9)
			So(missingHorizonErr, ShouldNotBeNil)
		})
	})

	Convey("Given only a ready direction forecast", t, func() {
		graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
		graph.Forecast = &learning.RLSOutput{Value: 0.4, Ready: true}
		graph.ForecastHorizon = 6

		value, confidence, sources, err := decisionPerspective(graph, 0.9)

		Convey("It should refuse to invent a return from the direction lean", func() {
			So(err, ShouldBeNil)
			So(value, ShouldEqual, 0)
			So(confidence, ShouldEqual, 0.9)
			So(sources, ShouldBeEmpty)
		})
	})
}

func BenchmarkDecisionPerspective(b *testing.B) {
	graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
	graph.Forecast = &learning.RLSOutput{Value: 0.01, Ready: true}
	graph.ForecastHorizon = 1
	graph.AddNode(&logicgraph.Node{
		ID: "causal:BTC/USD:doExpectation", Kind: logicgraph.KindCausal,
		Value: 0.02, Confidence: 0.8, Metadata: map[string]any{"horizon": 1},
	})

	for b.Loop() {
		if _, _, _, err := decisionPerspective(graph, 0.9); err != nil {
			b.Fatal(err)
		}
	}
}
