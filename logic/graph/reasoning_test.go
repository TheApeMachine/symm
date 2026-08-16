package graph

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/mcts"
)

func TestReasoningFrame(t *testing.T) {
	Convey("Given an empty evidence graph", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		frame := graph.ReasoningFrame()

		Convey("It should still produce a complete named intervention state", func() {
			So(mcts.ValidateReasoningFrame(frame), ShouldBeNil)
			maximumHorizon, found := frame.Get(mcts.SymbolMaximumHorizon)
			So(found, ShouldBeTrue)
			So(maximumHorizon, ShouldEqual, 1)
		})
	})
}

func TestApplyReasoningIntervention(t *testing.T) {
	Convey("Given a complete intervention state with supportive context", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		frame := graph.ReasoningFrame()
		frame.Put(mcts.SymbolContextConfidence, 1)
		frame.Put(mcts.SymbolFlow, 1)
		frame.Put(mcts.SymbolCoherence, 1)
		frame.Put(mcts.SymbolRegime, 1)

		Convey("Entering should advance the horizon and expose the position", func() {
			next, err := graph.ApplyReasoningIntervention(frame, mcts.ActionEnter)
			So(err, ShouldBeNil)
			horizon, _ := next.Get(mcts.SymbolHorizon)
			position, _ := next.Get(mcts.SymbolPosition)
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
