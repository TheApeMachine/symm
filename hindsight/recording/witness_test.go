package recording

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
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

			// Resident state and other witnesses share one table now, so both
			// halves of this assertion are predicates on artifact_kind rather
			// than reads of two separate key prefixes.
			states, err := engine.Witnesses(context.Background(), string(capture.Run), "state")
			So(err, ShouldBeNil)
			So(states, ShouldHaveLength, 1)
			So(states[0].Envelope.Sequence, ShouldEqual, int64(capture.Sequence))
			So(states[0].Payload, ShouldNotBeEmpty)

			decisions, err := engine.Witnesses(context.Background(), string(capture.Run), "decision")
			So(err, ShouldBeNil)
			So(decisions, ShouldHaveLength, 1)
		})

		Convey("Every precursor survives even when full-state witnesses are sampled", func() {
			envelope.CVD = data.NewMeasurement[float64]("cvd", nil)
			envelope.CVD.Label, envelope.CVD.At, envelope.CVD.From = "TEST/USD", time.Unix(2, 0), time.Unix(1, 0)
			for index := range 3 {
				envelope.CaptureOrdinal = uint64(index)
				envelope.CVD.Metrics["change"] = data.Metric[float64]{Label: "change", Raw: float64(index)}
				node.Step(envelope)
			}
			So(writer.Close(), ShouldBeNil)
			precursors, err := engine.Witnesses(t.Context(), string(capture.Run), "precursor")
			So(err, ShouldBeNil)
			So(len(precursors), ShouldEqual, 3)
			for _, precursor := range precursors {
				So(precursor.Boundary, ShouldEqual, "after-logic")
			}
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
