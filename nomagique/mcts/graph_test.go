package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type graphFixture struct {
	edgeValue   float64
	rootValue   float64
	targetValue float64
}

func (graphFixture) Roots() []string { return []string{"compression"} }

func (graphFixture) Targets(node string) []string {
	if node == "compression" {
		return []string{"ignition"}
	}

	return nil
}

func (fixture graphFixture) NodeValue(node string) (float64, float64) {
	if node == "ignition" {
		return fixture.targetValue, 0.9
	}

	return fixture.rootValue, 0.8
}

func (fixture graphFixture) EdgeValue(_, _ string) (float64, float64) {
	return fixture.edgeValue, 0.86
}

func TestNewGraphState(t *testing.T) {
	Convey("Given a directed weighted graph", t, func() {
		state, err := NewGraphState(graphFixture{edgeValue: 0.86})

		Convey("It should expose graph roots as search actions", func() {
			So(err, ShouldBeNil)
			So(state.GetPossibleActions(), ShouldResemble, []float64{0})
			So(state.History(), ShouldHaveLength, 1)
		})
	})
}

func TestGraphStateApplyAction(t *testing.T) {
	Convey("Given a graph search state", t, func() {
		state, err := NewGraphState(graphFixture{edgeValue: 0.86})
		So(err, ShouldBeNil)

		root := state.ApplyAction(0).(*GraphState)
		next := root.ApplyAction(0).(*GraphState)

		Convey("It should traverse weighted graph edges", func() {
			So(root.IsTerminal(), ShouldBeFalse)
			So(next.IsTerminal(), ShouldBeTrue)
			So(root.GetReward(), ShouldEqual, 0.0)
			So(next.GetReward(), ShouldAlmostEqual, 0.86*0.86)
			So(next.ToVector(), ShouldHaveLength, 3)
		})
	})

	Convey("Given equal evidence with the opposite relation sign", t, func() {
		supporting, supportErr := NewGraphState(graphFixture{edgeValue: 0.86})
		contradicting, contradictErr := NewGraphState(graphFixture{edgeValue: -0.86})
		So(supportErr, ShouldBeNil)
		So(contradictErr, ShouldBeNil)

		supportRoot := supporting.ApplyAction(0).(*GraphState)
		supportLeaf := supportRoot.ApplyAction(0).(*GraphState)
		contradictRoot := contradicting.ApplyAction(0).(*GraphState)
		contradictLeaf := contradictRoot.ApplyAction(0).(*GraphState)

		Convey("It should classify supporting and contradicting edge evidence by sign", func() {
			So(supportRoot.GetReward(), ShouldEqual, 0.0)
			So(contradictRoot.GetReward(), ShouldEqual, 0.0)
			So(supportLeaf.GetReward(), ShouldBeGreaterThan, 0.0)
			So(contradictLeaf.GetReward(), ShouldBeLessThan, 0.0)
		})
	})

	Convey("Given identical edge evidence attached to incompatible raw node scales", t, func() {
		small, smallErr := NewGraphState(graphFixture{
			edgeValue: 0.5, rootValue: 0.0001, targetValue: 0.0002,
		})
		large, largeErr := NewGraphState(graphFixture{
			edgeValue: 0.5, rootValue: 1e12, targetValue: -1e15,
		})
		So(smallErr, ShouldBeNil)
		So(largeErr, ShouldBeNil)

		smallReward := small.ApplyAction(0).(*GraphState).ApplyAction(0).GetReward()
		largeReward := large.ApplyAction(0).(*GraphState).ApplyAction(0).GetReward()

		Convey("Raw price and return units should not contaminate graph evidence", func() {
			So(smallReward, ShouldEqual, largeReward)
		})
	})
}

func TestGraphStateSearch(t *testing.T) {
	Convey("Given a reusable graph state", t, func() {
		state, err := NewGraphState(graphFixture{edgeValue: 0.86})
		So(err, ShouldBeNil)
		history := state.History()
		engine := NewCausalMCTS(
			DefaultCausalEngine{},
			1,
			1,
			0,
			GraphTreatmentColumn,
			GraphTargetColumn,
			GraphControlColumns,
			GraphFeatureColumns,
			true,
		)

		root, action, err := engine.Search(state, len(history), history)

		Convey("It should search the graph directly", func() {
			So(err, ShouldBeNil)
			So(root.Children, ShouldHaveLength, 1)
			So(action, ShouldEqual, 0)
		})
	})
}

func BenchmarkGraphStateApplyAction(b *testing.B) {
	state, err := NewGraphState(graphFixture{edgeValue: 0.86})

	if err != nil {
		b.Fatal(err)
	}

	root := state.ApplyAction(0).(*GraphState)

	for b.Loop() {
		_ = root.ApplyAction(0)
	}
}
