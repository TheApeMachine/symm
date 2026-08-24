package types

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReasoningFrame(t *testing.T) {
	Convey("Given an empty evidence graph", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		frame := graph.ReasoningFrame()

		Convey("It should still produce a complete named intervention state", func() {
			So(ValidateReasoningFrame(frame), ShouldBeNil)
			maximumHorizon, found := frame.Get(SymbolMaximumHorizon)
			So(found, ShouldBeTrue)
			So(maximumHorizon, ShouldEqual, 1)
		})
	})
}

func TestReasoningFramePredictiveDynamics(t *testing.T) {
	Convey("Given continuous predictive dynamics in the evidence graph", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.AddNode(&Node{
			ID:         "res:BTC/USD:generalized_velocity",
			Symbol:     "BTC/USD",
			Source:     "resonance_dynamics",
			Kind:       KindResonance,
			Value:      0.4,
			Confidence: 1,
		})
		graph.AddNode(&Node{
			ID:         "res:BTC/USD:passivity_residue",
			Symbol:     "BTC/USD",
			Source:     "resonance_dynamics",
			Kind:       KindResonance,
			Value:      0.2,
			Confidence: 1,
		})
		graph.AddNode(&Node{
			ID:         "res:BTC/USD:jump_variance",
			Symbol:     "BTC/USD",
			Source:     "resonance_dynamics",
			Kind:       KindResonance,
			Value:      0.4,
			Confidence: 1,
		})
		frame := graph.ReasoningFrame()
		flow, _ := frame.Get(SymbolFlow)
		surprise, _ := frame.Get(SymbolSurprise)

		Convey("It should route motion into flow and physical residue into risk context", func() {
			So(flow, ShouldEqual, 0.4)
			So(surprise, ShouldAlmostEqual, 0.3, 0.0001)
		})
	})
}

func TestApplyReasoningIntervention(t *testing.T) {
	Convey("Given a complete intervention state with supportive context", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		frame := graph.ReasoningFrame()
		frame.Put(SymbolContextConfidence, 1)
		frame.Put(SymbolFlow, 1)
		frame.Put(SymbolCoherence, 1)
		frame.Put(SymbolRegime, 1)

		Convey("Entering should advance the horizon and expose the position", func() {
			next, err := graph.ApplyReasoningIntervention(frame, ReasoningActionEnter)
			So(err, ShouldBeNil)
			horizon, _ := next.Get(SymbolHorizon)
			position, _ := next.Get(SymbolPosition)
			So(horizon, ShouldEqual, 1)
			So(position, ShouldEqual, 1)
		})
	})
}

func TestReasoningTopology(t *testing.T) {
	Convey("Given a market graph payload", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		topology := graph.ReasoningTopology()

		Convey("It should expose the treatment, mediator, and target chain", func() {
			So(topology.Treatment, ShouldContainSubstring, "treatment")
			So(topology.Mediator, ShouldContainSubstring, "mediator")
			So(topology.Target, ShouldContainSubstring, "target")
			So(topology.Nodes, ShouldHaveLength, 3)
			So(topology.Links, ShouldHaveLength, 2)
		})

		Convey("It should remain serializable for the graph UI", func() {
			encoded, err := json.Marshal(graph)
			So(err, ShouldBeNil)
			So(string(encoded), ShouldContainSubstring, "reasoning")
		})
	})
}
