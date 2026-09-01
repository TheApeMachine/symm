package cmd

import (
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

/*
witnessNode records what the live pipeline observed and produced. It never
feeds a value back into trading.
*/
type witnessNode struct {
	writer         *store.Writer
	asyncWriter    *store.AsyncWitnessWriter
	categorySolver *category.Solver
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
	node.write(hindsight.ArtifactWitness{
		Envelope: ref,
		Boundary: "observe",
		Artifact: hindsight.ArtifactID{
			Kind: "state",
			Identity: string(ref.Origin.Run) + ":" +
				strconv.FormatUint(uint64(ref.Origin.Sequence), 10) + ":" +
				strconv.FormatUint(ref.Ordinal, 10),
		},
		Payload: envelope.EncodeBytes(),
	})

	for _, categoryReading := range envelope.Categories {
		node.record(ref, "after-category", "category", string(categoryReading.Type), "category", node.categoryVersion())
	}

	for _, measurement := range envelope.SignalMeasurements() {
		if measurement == nil {
			continue
		}

		node.record(ref, "after-signals", "measurement", measurement.ID, "measurements", 0)
	}

	if envelope.StrategyRound == nil {
		return envelope
	}

	for _, decision := range envelope.StrategyRound.Decisions {
		node.recordDecision(ref, decision)
	}

	return envelope
}

func (node witnessNode) recordDecision(ref hindsight.EnvelopeRef, decision *types.Decision) {
	if decision == nil || decision.ID == "" {
		return
	}

	semanticParents := make([]string, 0, len(decision.PerspectiveSources)+1)

	for _, source := range decision.PerspectiveSources {
		semanticParents = append(semanticParents, source.Source)
	}

	if decision.CausalIdentification != "" {
		semanticParents = append(semanticParents, decision.CausalIdentification)
	}

	node.write(hindsight.ArtifactWitness{
		Envelope:              ref,
		Boundary:              "after-strategy",
		Artifact:              hindsight.ArtifactID{Kind: "decision", Identity: decision.ID},
		Component:             "strategy",
		ComponentStateVersion: decision.CalibrationCount,
		ImmediateParents:      []hindsight.EnvelopeRef{ref},
		SemanticParents:       semanticParents,
	})
}

func (node witnessNode) record(
	ref hindsight.EnvelopeRef,
	boundary string,
	kind string,
	identity string,
	component string,
	version uint64,
) {
	if identity == "" {
		return
	}

	node.write(hindsight.ArtifactWitness{
		Envelope:              ref,
		Boundary:              boundary,
		Artifact:              hindsight.ArtifactID{Kind: kind, Identity: identity},
		Component:             component,
		ComponentStateVersion: version,
		ImmediateParents:      []hindsight.EnvelopeRef{ref},
	})
}

func (node witnessNode) categoryVersion() uint64 {
	if node.categorySolver == nil {
		return 0
	}

	return node.categorySolver.Version()
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
