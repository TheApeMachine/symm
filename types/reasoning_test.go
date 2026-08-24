package types

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
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

func TestValidateReasoningFrameFiniteHorizon(t *testing.T) {
	Convey("Given a reasoning frame", t, func() {
		Convey("a NaN max_horizon is rejected", func() {
			frame := completeValidationFrame()
			frame.Put(SymbolMaximumHorizon, math.NaN())
			frame.Put(SymbolHorizon, 0)

			err := ValidateReasoningFrame(frame)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "max_horizon must be finite")
		})

		Convey("a positive-infinity max_horizon is rejected", func() {
			frame := completeValidationFrame()
			frame.Put(SymbolMaximumHorizon, math.Inf(1))
			frame.Put(SymbolHorizon, 0)

			err := ValidateReasoningFrame(frame)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "max_horizon must be finite")
		})

		Convey("a NaN horizon is rejected", func() {
			frame := completeValidationFrame()
			frame.Put(SymbolMaximumHorizon, 5)
			frame.Put(SymbolHorizon, math.NaN())

			err := ValidateReasoningFrame(frame)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "horizon must be finite")
		})

		Convey("a positive-infinity horizon is rejected", func() {
			frame := completeValidationFrame()
			frame.Put(SymbolMaximumHorizon, 5)
			frame.Put(SymbolHorizon, math.Inf(1))

			err := ValidateReasoningFrame(frame)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "horizon must be finite")
		})

		Convey("valid finite horizons within range remain accepted", func() {
			frame := completeValidationFrame()
			frame.Put(SymbolMaximumHorizon, 5)
			frame.Put(SymbolHorizon, 3)

			So(ValidateReasoningFrame(frame), ShouldBeNil)
		})
	})
}

/*
completeValidationFrame builds a legacy frame with every required symbol so
validation reaches the horizon checks.
*/
func completeValidationFrame() nmtypes.Frame {
	frame := nmtypes.Frame{}
	frame.Put(SymbolMaximumHorizon, 5)
	frame.Put(SymbolHorizon, 0)
	frame.Put(SymbolTreatment, ReasoningActionWait)
	frame.Put(SymbolTarget, 0)
	frame.Put(SymbolPosition, 0)
	frame.Put(SymbolContextConfidence, 0)
	frame.Put(SymbolFlow, 0)
	frame.Put(SymbolLiquidityImpact, 0)
	frame.Put(SymbolHawkes, 0)
	frame.Put(SymbolCoherence, 0)
	frame.Put(SymbolRegime, 0)
	frame.Put(SymbolSurprise, 0)
	frame.Put(SymbolArchetype, 0)
	frame.Put(SymbolVelocity, 0)
	frame.Put(SymbolStoredEnergy, 0)
	frame.Put(SymbolCausalExpectation, 0)
	frame.Put(SymbolSpread, 0)

	return frame
}
