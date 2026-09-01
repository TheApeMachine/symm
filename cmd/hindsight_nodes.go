package cmd

import (
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

/*
witnessNode records what the live pipeline observed and produced. It never
feeds a value back into trading.
*/
type witnessNode struct {
	writer      *store.Writer
	asyncWriter *store.AsyncWitnessWriter
}

func (node witnessNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || (node.writer == nil && node.asyncWriter == nil) {
		return envelope
	}

	if !envelope.CaptureID.Valid() {
		return envelope
	}

	ref := hindsight.EnvelopeRef{
		Origin:  envelope.CaptureID,
		Ordinal: envelope.CaptureOrdinal,
	}
	if !hasActionableDecision(envelope.StrategyRound) {
		return envelope
	}

	node.write(hindsight.ArtifactWitness{
		Envelope: ref,
		Boundary: "after-strategy",
		Artifact: hindsight.ArtifactID{
			Kind: "state",
			Identity: string(ref.Origin.Run) + ":" +
				strconv.FormatUint(uint64(ref.Origin.Sequence), 10) + ":" +
				strconv.FormatUint(ref.Ordinal, 10),
		},
		Payload: envelope.EncodeBytes(),
	})

	for _, decision := range envelope.StrategyRound.Decisions {
		node.recordDecision(ref, decision)
	}

	return envelope
}

func hasActionableDecision(round *types.StrategyRound) bool {
	if round == nil {
		return false
	}

	for _, decision := range round.Decisions {
		if decision != nil && decision.Action != types.ActionNothing {
			return true
		}
	}

	return false
}

func (node witnessNode) recordDecision(ref hindsight.EnvelopeRef, decision *types.Decision) {
	if decision == nil || decision.ID == "" {
		return
	}

	node.write(hindsight.ArtifactWitness{
		Envelope:              ref,
		Boundary:              "after-strategy",
		Artifact:              hindsight.ArtifactID{Kind: "decision", Identity: decision.ID},
		Component:             "strategy",
		ComponentStateVersion: decision.CalibrationCount,
		ImmediateParents:      []hindsight.EnvelopeRef{ref},
	})
}

func (node witnessNode) write(witness hindsight.ArtifactWitness) {
	if node.asyncWriter != nil {
		node.asyncWriter.Enqueue(witness)
		return
	}

	_ = node.writer.WriteWitness(witness)
}

/*
hindsightLifecycleRecorder persists Desk lifecycle facts without influencing
the lifecycle transition itself.
*/
type hindsightLifecycleRecorder struct {
	engine *store.SQLite
	runID  hindsight.RunID
}

func (recorder hindsightLifecycleRecorder) RecordLifecycle(event hindsight.LifecycleEvent) {
	if recorder.engine == nil || event.DecisionID == "" || event.Kind == "" {
		return
	}

	if err := recorder.engine.WriteLifecycleEvent(recorder.runID, event); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "symm: record lifecycle event", err))
	}
}
