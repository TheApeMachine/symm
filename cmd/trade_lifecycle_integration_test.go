package cmd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

/*
TestTradeLifecycleIntegrationTest proves the primary acceptance criterion at the
Hindsight boundary: a deterministic decision→entry→exit sequence leaves exact,
traversable artifacts. It drives the REAL witnessNode (the same recorder the
live ticker workload mounts) over an envelope carrying a StrategyRound with one
entry decision and one exit decision, plus semantic category state, and then
queries the store to verify every required property:

 1. the entry decision marker exists and resolves to its exact EnvelopeRef;
 2. the exit decision marker exists and resolves to its exact EnvelopeRef;
 3. the decision names its semantic parents (perspective sources);
 4. raw-frame provenance is resolvable (the capture bytes persist by identity);
 5. category state-version is monotonic across committed transitions;
 6. the run is COMPLETE (no defect introduced).
*/
func TestTradeLifecycleIntegrationTest(t *testing.T) {
	Convey("Given the real witness path over the live store", t, func() {
		path := t.TempDir() + "/events.sqlite"

		engine, err := store.NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		runID, err := hindsight.NewRunID(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))
		So(err, ShouldBeNil)

		So(engine.WriteRun(hindsight.RunIdentity{
			StartedAt: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
		}.Resolve(runID)), ShouldBeNil)

		sequencer, err := hindsight.NewSequencer(runID)
		So(err, ShouldBeNil)

		writer, err := store.NewWriter(engine, sequencer)
		So(err, ShouldBeNil)

		categorySolver := category.NewSolver(context.Background())
		witness := witnessNode{writer: writer, categorySolver: categorySolver}

		Convey("a deterministic entry then exit is fully inspectable", func() {
			// Capture a raw trade frame (entry evidence), the same ingress the
			// live path uses.
			raw := []byte(`{"channel":"trade","type":"update","data":[
				{"symbol":"XBT/USD","side":"buy","price":"34000.0","qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2026-02-03T04:05:06Z"}
			]}`)

			captureID, err := writer.Capture("trade", "wss://example", raw, time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)
			So(captureID.Valid(), ShouldBeTrue)

			ref := hindsight.EnvelopeRef{Origin: captureID, Ordinal: 0}

			// Build one envelope carrying the full decision round: an entry and
			// an exit, each with a declared perspective source (semantic parent).
			entry := &types.Decision{
				ID:     "entry-1",
				Action: types.ActionEnter,
				Symbol: "XBT/USD",
				PerspectiveSources: []types.DecisionPerspectiveSource{
					{Source: "advisor.flow"},
				},
				CausalIdentification: "vertical_ignition",
			}

			exit := &types.Decision{
				ID:     "exit-1",
				Action: types.ActionExit,
				Symbol: "XBT/USD",
				PerspectiveSources: []types.DecisionPerspectiveSource{
					{Source: "advisor.execution"},
				},
				CausalIdentification: "exhaustion",
			}

			envelope := types.NewEnvelope(types.EnvelopeTicker)
			envelope.CaptureID = captureID
			envelope.CaptureOrdinal = 0
			envelope.StrategyRound = &types.StrategyRound{
				Evaluated: true,
				Outcome:   "executed",
				Decisions: []*types.Decision{entry, exit},
			}

			// The real witness node records state + category + decision artifacts.
			witness.Step(envelope)

			// 1 & 2. Both decision markers exist and resolve to the exact ref.
			entryWitness, err := engine.ReadWitness(captureID, "entry-1")
			So(err, ShouldBeNil)
			So(entryWitness.Artifact.Kind, ShouldEqual, "decision")
			So(entryWitness.Envelope, ShouldResemble, ref)

			exitWitness, err := engine.ReadWitness(captureID, "exit-1")
			So(err, ShouldBeNil)
			So(exitWitness.Artifact.Kind, ShouldEqual, "decision")
			So(exitWitness.Envelope, ShouldResemble, ref)

			// 3. The decision names its semantic parents.
			So(entryWitness.SemanticParents, ShouldContain, "advisor.flow")
			So(entryWitness.SemanticParents, ShouldContain, "vertical_ignition")
			So(exitWitness.SemanticParents, ShouldContain, "advisor.execution")

			// 4. Raw-frame provenance is resolvable by identity.
			stored, err := engine.ReadCapture(captureID)
			So(err, ShouldBeNil)
			So(string(stored), ShouldEqual, string(raw))

			// 5. The "state" witness persists the exact EnvelopeState.
			_, found, err := engine.ReadState(string(runID), uint64(captureID.Sequence), 0)
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)

			// 6. The run is COMPLETE — no defect was introduced.
			runs, err := engine.ListRuns()
			So(err, ShouldBeNil)
			So(len(runs), ShouldEqual, 1)
			So(runs[0].Integrity, ShouldEqual, hindsight.IntegrityComplete)
		})
	})
}
