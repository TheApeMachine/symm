package manifold

import (
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
runtimeControls derives the per-step Metal controls from static engine config and
the latest Hawkes excitation outcome. Event chronology controls the solver clock;
Hawkes decay controls damping rather than being misused as a spatial speed.
*/
func runtimeControls(
	config pmanifold.Config,
	outcome excitation.Outcome,
	interval time.Duration,
) pmanifold.RuntimeControls {
	controls := config.RuntimeControls()
	controls.DeltaT = integrationDeltaT(config, interval)

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
