package cmd

import (
	"strconv"
	"time"

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
	writer        *store.Writer
	asyncWriter   *store.AsyncWitnessWriter
	phases        map[opportunityWitnessKey]types.OpportunityPhase
	lastWitnessed map[string]time.Time
}

type opportunityWitnessKey struct {
	symbol    string
	archetype types.OpportunityArchetype
}

func newWitnessNode(
	writer *store.Writer,
	asyncWriter *store.AsyncWitnessWriter,
) *witnessNode {
	return &witnessNode{
		writer:        writer,
		asyncWriter:   asyncWriter,
		phases:        make(map[opportunityWitnessKey]types.OpportunityPhase),
		lastWitnessed: make(map[string]time.Time),
	}
}

func (node *witnessNode) Step(envelope *types.Envelope) *types.Envelope {
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

	if !node.shouldWitness(envelope) {
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

	if envelope.StrategyRound != nil {
		for _, decision := range envelope.StrategyRound.Decisions {
			node.recordDecision(ref, decision)
		}
	}

	return envelope
}

func (node *witnessNode) shouldWitness(envelope *types.Envelope) bool {
	symbol := envelopeSymbol(envelope)

	if hasActionableDecision(envelope.StrategyRound) {
		if symbol != "" {
			node.lastWitnessed[symbol] = time.Now()
		}

		return true
	}

	changed := false

	for _, candidate := range envelope.Opportunities {
		if candidate == nil || candidate.Symbol == "" || candidate.Archetype == "" {
			continue
		}

		key := opportunityWitnessKey{
			symbol:    candidate.Symbol,
			archetype: candidate.Archetype,
		}

		if node.phases[key] != candidate.Phase {
			node.phases[key] = candidate.Phase
			changed = true
		}
	}

	if changed {
		if symbol != "" {
			node.lastWitnessed[symbol] = time.Now()
		}

		return true
	}

	if symbol == "" {
		return false
	}

	hasState := envelope.StrategyRound != nil ||
		len(envelope.Perspectives) > 0 ||
		len(envelope.Categories) > 0 ||
		len(envelope.Opportunities) > 0 ||
		envelope.PumpDump != nil ||
		envelope.CVD != nil

	if !hasState {
		return false
	}

	last, exists := node.lastWitnessed[symbol]

	if !exists || time.Since(last) >= time.Second {
		node.lastWitnessed[symbol] = time.Now()
		return true
	}

	return false
}

func envelopeSymbol(envelope *types.Envelope) string {
	if envelope == nil {
		return ""
	}

	if envelope.Key != "" {
		return envelope.Key
	}

	if envelope.TickerData.Symbol != "" {
		return envelope.TickerData.Symbol
	}

	if envelope.TradeData.Symbol != "" {
		return envelope.TradeData.Symbol
	}

	if envelope.Level3Data.Symbol != "" {
		return envelope.Level3Data.Symbol
	}

	if envelope.StrategyRound != nil && envelope.StrategyRound.Symbol != "" {
		return envelope.StrategyRound.Symbol
	}

	if len(envelope.Opportunities) > 0 && envelope.Opportunities[0] != nil && envelope.Opportunities[0].Symbol != "" {
		return envelope.Opportunities[0].Symbol
	}

	if len(envelope.Perspectives) > 0 {
		return envelope.Perspectives[0].Symbol
	}

	return ""
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

func (node *witnessNode) recordDecision(ref hindsight.EnvelopeRef, decision *types.Decision) {
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

func (node *witnessNode) write(witness hindsight.ArtifactWitness) {
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
	writer *store.Writer
	runID  hindsight.RunID
}

func (recorder hindsightLifecycleRecorder) RecordLifecycle(event hindsight.LifecycleEvent) {
	if recorder.writer == nil || event.DecisionID == "" || event.Kind == "" {
		return
	}

	if err := recorder.writer.WriteLifecycle(recorder.runID, event); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "symm: record lifecycle event", err))
	}
}
