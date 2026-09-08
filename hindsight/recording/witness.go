package recording

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (node *Session) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || node == nil {
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

	node.record(hindsight.ArtifactWitness{
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

func (node *Session) shouldWitness(envelope *types.Envelope) bool {
	symbol := envelope.Symbol()

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

func (node *Session) recordDecision(ref hindsight.EnvelopeRef, decision *types.Decision) {
	if decision == nil || decision.ID == "" {
		return
	}

	node.record(hindsight.ArtifactWitness{
		Envelope:              ref,
		Boundary:              "after-strategy",
		Artifact:              hindsight.ArtifactID{Kind: "decision", Identity: decision.ID},
		Component:             "strategy",
		ComponentStateVersion: decision.CalibrationCount,
		ImmediateParents:      []hindsight.EnvelopeRef{ref},
	})
}

func (node *Session) record(witness hindsight.ArtifactWitness) {
	node.mutex.Lock()
	defer node.mutex.Unlock()
	family := "witnesses"
	if witness.Artifact.Kind == "state" {
		family = "states"
	}
	key := witness.Envelope.Key(family)
	if family == "witnesses" {
		key = strings.TrimSuffix(key, ".json") + "/" + url.PathEscape(witness.Artifact.Identity) + ".json"
	}
	if err := node.enqueue(key, witness); err != nil {
		errnie.Error(err)
	}
}
