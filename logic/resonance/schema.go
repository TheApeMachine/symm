package resonance

import (
	"sort"

	"github.com/theapemachine/symm/types"
)

const manifoldContextPrefix = "manifold-context"

type featureSchema struct {
	identities []string
	known      map[string]struct{}
}

/*
taskSchema returns the complete semantic input surface for the direction task.
It is fixed for a symbol's lifetime: asynchronously arriving signal families
populate known coordinates instead of rebuilding the predictive coder and
throwing away its task calibration every time a new metric first appears.
*/
func taskSchema(symbol string) *featureSchema {
	identities := make([]string, 0)

	for _, source := range types.SignalSources {
		groups := types.SignalMetricGroups[source]
		keys := make([]string, 0, len(groups))

		for key := range groups {
			if types.ResonanceMetricAllowed(source, key) {
				keys = append(keys, key)
			}
		}

		sort.Strings(keys)

		for _, key := range keys {
			identities = append(
				identities,
				string(source)+":"+symbol+":"+key,
			)
		}
	}

	for _, key := range manifoldContextKeys {
		identities = append(identities, manifoldContextIdentity(symbol, key))
	}

	known := make(map[string]struct{}, len(identities))

	for _, identity := range identities {
		known[identity] = struct{}{}
	}

	return &featureSchema{identities: identities, known: known}
}

var manifoldContextKeys = []string{
	"pressure_gradient",
	"divergence",
	"coherence",
	"guidance_speed",
	"viscosity",
	"phase_direction",
	"phase_confidence",
	"phase_support",
	"phase_contradiction",
	"phase_balance",
}

func manifoldContextIdentity(symbol, key string) string {
	return manifoldContextPrefix + ":" + symbol + ":" + key
}

/*
manifoldContextReadings exposes the universe-wide manifold and phase inference
as explicit context. These values do not masquerade as per-symbol price
indicators; every symbol receives the same physical-universe reading in its own
stable coordinates so the predictive coder can learn when local precursors are
conditioned by a wider regime.
*/
func manifoldContextReadings(thesis *types.Thesis, symbol string) map[string]float64 {
	readings := make(map[string]float64)

	if manifold, found := thesis.ManifoldSnapshot(); found && manifold.IsFinite() {
		readings[manifoldContextIdentity(symbol, "pressure_gradient")] = manifold.PressureGradNorm
		readings[manifoldContextIdentity(symbol, "divergence")] = manifold.Divergence
		readings[manifoldContextIdentity(symbol, "coherence")] = manifold.CoherenceMag2
		readings[manifoldContextIdentity(symbol, "guidance_speed")] = manifold.GuidanceSpeed
		readings[manifoldContextIdentity(symbol, "viscosity")] = manifold.ViscosityProxy
	}

	if phase, found := thesis.PhaseSnapshot(); found {
		if inference, ready := phase.Inference(); ready {
			readings[manifoldContextIdentity(symbol, "phase_direction")] = inference.Direction
			readings[manifoldContextIdentity(symbol, "phase_confidence")] = inference.Confidence
			readings[manifoldContextIdentity(symbol, "phase_support")] = inference.Support
			readings[manifoldContextIdentity(symbol, "phase_contradiction")] = inference.Contradiction
			readings[manifoldContextIdentity(symbol, "phase_balance")] = inference.Balance
		}
	}

	return readings
}
