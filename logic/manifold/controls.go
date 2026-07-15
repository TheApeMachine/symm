package manifold

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
runtimeControls derives the per-step Metal controls from static engine config and
the latest Hawkes excitation outcome so GPE self-interaction and wave damping track
empirical reflexivity and decay.
*/
func runtimeControls(
	config pmanifold.Config,
	outcome excitation.Outcome,
) pmanifold.RuntimeControls {
	controls := config.RuntimeControls()
	controls.DeltaT = integrationDeltaT(config, outcome)

	if !outcome.Readiness.HawkesFit {
		return controls
	}

	hawkesEta := outcome.Fit.SpectralRadius
	hawkesBeta := outcome.Fit.Beta

	if hawkesEta <= 0 || hawkesBeta <= 0 {
		return controls
	}

	controls.GInteraction = config.GInteraction() * hawkesEta
	controls.EnergyDecay = hawkesBeta

	return controls
}
