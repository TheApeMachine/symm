package recording

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

func TestSessionStep(t *testing.T) {
	Convey("Given a non-action opportunity phase transition", t, func() {
		writer, engine := newSessionFixture(t, "recording-witness")

		capture, err := writer.Capture(
			"ticker", "public", []byte(`{"channel":"ticker"}`), time.Unix(2, 0),
			hindsight.StreamRef{Stream: "public:ticker", Epoch: 1, Sequence: 1},
		)
		So(err, ShouldBeNil)

		decision := &types.Decision{Action: types.ActionNothing, Symbol: "TEST/USD"}
		decision.EnsureID()
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
		node := writer

		Convey("the phase and its non-action decision are persisted for evaluation", func() {
			So(node.Step(envelope), ShouldEqual, envelope)

			So(writer.Close(), ShouldBeNil)
			stateWitness, found, err := store.Find(context.Background(), engine, capture.Run.Prefix("states"), func(witness hindsight.ArtifactWitness) bool {
				return witness.Envelope == (hindsight.EnvelopeRef{Origin: capture})
			})
			So(found, ShouldBeTrue)
			So(err, ShouldBeNil)
			So(stateWitness.Artifact.Kind, ShouldEqual, "state")
			So(stateWitness.Payload, ShouldNotBeEmpty)

			decisions, err := store.List[hindsight.ArtifactWitness](context.Background(), engine, capture.Run.Prefix("witnesses"))
			So(decisions, ShouldHaveLength, 1)
			decisionWitness := decisions[0]
			So(err, ShouldBeNil)
			So(decisionWitness.Artifact.Kind, ShouldEqual, "decision")
		})

		Convey("the same phase does not create repeated full-state witnesses", func() {
			So(node.shouldWitness(envelope), ShouldBeTrue)
			So(node.shouldWitness(envelope), ShouldBeFalse)
		})

		Convey("periodic heartbeat witnesses state after elapsed interval", func() {
			So(node.shouldWitness(envelope), ShouldBeTrue)
			So(node.shouldWitness(envelope), ShouldBeFalse)

			node.lastWitnessed["TEST/USD"] = time.Now().Add(-2 * time.Second)
			So(node.shouldWitness(envelope), ShouldBeTrue)
		})
	})
}
