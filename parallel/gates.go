package parallel

import (
	"math"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

type GateSnapshot struct {
	Coherence bool
	Familiar  bool
	Guidance  bool
	Metabolic bool
	AllPass   bool
	Reason    string
}

func evaluateGates(
	reading sensorium.Reading,
	nullReading sensorium.Reading,
	fieldDeltaR float64,
	phase SymbolPhase,
	forecastSign float64,
	fieldHealthy bool,
) GateSnapshot {
	snap := GateSnapshot{}

	snap.Coherence = phase.DeltaR > 0.01 || fieldDeltaR > 0.005
	snap.Familiar = phase.Stable

	guidanceSign := 0.0
	if phase.Guidance > 0 {
		guidanceSign = 1
	} else if phase.Guidance < 0 {
		guidanceSign = -1
	}

	snap.Guidance = forecastSign == 0 || guidanceSign == forecastSign

	aboveVacuum := reading.KuramotoR >= nullReading.KuramotoR*0.85
	snap.Metabolic = fieldHealthy && reading.Health.IsFinite() && phase.Heat > 0.15 && aboveVacuum

	snap.AllPass = snap.Coherence && snap.Familiar && snap.Guidance && snap.Metabolic

	if !snap.Coherence {
		snap.Reason = "coherence"
	} else if !snap.Familiar {
		snap.Reason = "familiarity"
	} else if !snap.Guidance {
		snap.Reason = "guidance"
	} else if !snap.Metabolic {
		snap.Reason = "metabolic"
	}

	return snap
}

func curveClearsFriction(curveMean, hurdle float64) bool {
	if hurdle <= 0 {
		return curveMean > 0
	}

	return curveMean > hurdle
}

func jeffreysReady(wins, losses float64) bool {
	alpha := wins + 1.5
	beta := losses + 1.5
	lower := (alpha - 1) / (alpha + beta - 2)

	return lower > 0.5
}

func skillReady(side CoderSide) bool {
	if !side.Calibrated {
		return false
	}

	wins := side.Skill * 10
	losses := (1 - side.Skill) * 10

	return jeffreysReady(wins, losses)
}

func physicsOnlyEnter(phase SymbolPhase, gates GateSnapshot) bool {
	if !gates.Metabolic || !gates.Coherence {
		return false
	}

	return phase.Guidance > 0 && phase.DeltaR > 0 && math.Abs(phase.Guidance) > 0.05
}
