package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

func TestWitnessNodeStep(t *testing.T) {
	Convey("Given a non-action opportunity phase transition", t, func() {
		engine, err := store.NewSQLite(t.TempDir() + "/hindsight.sqlite")
		So(err, ShouldBeNil)
		Reset(func() { _ = engine.Close() })

		runID, err := hindsight.NewRunID(time.Unix(1, 0))
		So(err, ShouldBeNil)
		sequencer, err := hindsight.NewSequencer(runID)
		So(err, ShouldBeNil)
		writer, err := store.NewWriter(engine, sequencer)
		So(err, ShouldBeNil)
		capture, err := writer.Capture(
			"ticker", "public", []byte(`{"channel":"ticker"}`), time.Unix(2, 0),
			hindsight.StreamRef{Stream: "public:ticker", Epoch: 1, Sequence: 1},
		)
		So(err, ShouldBeNil)

		decision := types.NewDecision(types.ActionNothing, "TEST/USD")
		decision.At = time.Unix(2, 0)
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.CaptureID = capture
		envelope.Opportunities = []*types.OpportunityCandidate{{
			Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
			Phase: types.PhaseForming, Direction: types.DirectionLong,
		}}
		envelope.StrategyRound = &types.StrategyRound{
			Symbol: "TEST/USD", Evaluated: true,
			Decisions: []*types.Decision{decision},
		}
		node := newWitnessNode(writer, nil)

		Convey("the phase and its non-action decision are persisted for evaluation", func() {
			So(node.Step(envelope), ShouldEqual, envelope)

			stateID := string(capture.Run) + ":1:0"
			stateWitness, err := engine.ReadWitness(capture, stateID)
			So(err, ShouldBeNil)
			So(stateWitness.Artifact.Kind, ShouldEqual, "state")
			So(stateWitness.Payload, ShouldNotBeEmpty)

			decisionWitness, err := engine.ReadWitness(capture, decision.ID)
			So(err, ShouldBeNil)
			So(decisionWitness.Artifact.Kind, ShouldEqual, "decision")
		})

		Convey("the same phase does not create repeated full-state witnesses", func() {
			So(node.shouldWitness(envelope), ShouldBeTrue)
			So(node.shouldWitness(envelope), ShouldBeFalse)
		})
	})
}
