package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

type interventionGraphFixture struct{}

func (interventionGraphFixture) ReasoningFrame() nomagique.Frame {
	return completeReasoningFrame()
}

func (interventionGraphFixture) ReasoningHistory() []nomagique.Frame {
	return nil
}

func (interventionGraphFixture) ReasoningKey() string {
	return "BTC/USD"
}

func (interventionGraphFixture) ApplyReasoningIntervention(
	state nomagique.Frame,
	action float64,
) (nomagique.Frame, error) {
	horizon, _ := state.Get(SymbolHorizon)
	position, _ := state.Get(SymbolPosition)
	state.Put(SymbolHorizon, horizon+1)
	state.Put(SymbolTreatment, action)
	state.Put(SymbolTarget, action)

	if action == ActionEnter {
		state.Put(SymbolPosition, 1)
	}

	if action == ActionExit {
		state.Put(SymbolPosition, 0)
	}

	if action == ActionScale {
		state.Put(SymbolPosition, position+1)
	}

	return state, nil
}

func TestNewGraphState(t *testing.T) {
	Convey("Given an intervention-capable market graph", t, func() {
		state := NewGraphState(interventionGraphFixture{})
		So(state.Err(), ShouldBeNil)

		Convey("Actions should describe strategic interventions, not graph nodes", func() {
			So(state.GetPossibleActions(), ShouldResemble, []float64{ActionWait, ActionEnter})
			nextState := state.ApplyAction(ActionEnter).(*GraphState)
			So(nextState.Err(), ShouldBeNil)
			So(nextState.GetPossibleActions(), ShouldResemble, []float64{
				ActionExit, ActionWait, ActionScale,
			})
			So(nextState.HistoryKey(), ShouldEqual, "BTC/USD")
		})
	})
}

type causalEngineFixture struct{}

func (causalEngineFixture) DoExpectation(
	history [][]float64,
	target int,
	minimumRows int,
	treatment int,
	level float64,
	controls []int,
) (float64, error) {
	return level, nil
}

func (causalEngineFixture) AbductiveCounterfactual(
	history [][]float64,
	target int,
	minimumRows int,
	features []int,
	linear bool,
	actualRow []float64,
	treatment int,
	level float64,
) (counterfactual float64, noise float64, err error) {
	return level, 0, nil
}

func TestSearch(t *testing.T) {
	Convey("Given a deterministic causal intervention search", t, func() {
		state := NewGraphState(interventionGraphFixture{})
		row, err := FrameToRow(completeReasoningFrame())
		So(err, ShouldBeNil)
		search := NewCausalMCTSWithSeed(
			causalEngineFixture{},
			1,
			1,
			1,
			ColumnTreatment,
			ColumnTarget,
			DefaultControlColumns,
			DefaultFeatureColumns,
			true,
			1,
		)

		Convey("It should expose observed, causal, and counterfactual provenance", func() {
			root, action, err := search.Search(state, 12, [][]float64{row})
			So(err, ShouldBeNil)
			So(root.Children, ShouldHaveLength, 2)
			So(ActionName(action), ShouldBeIn, "enter", "wait")

			selected := 0
			counterfactualMass := 0.0

			for _, child := range root.Children {
				if child.Selected {
					selected++
				}

				counterfactualMass += child.CounterfactualMass
			}

			So(selected, ShouldEqual, 1)
			So(counterfactualMass, ShouldBeGreaterThan, 0)
			So(root.Trace().Children, ShouldHaveLength, 2)
		})
	})
}
