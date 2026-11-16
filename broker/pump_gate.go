package broker

import (
	"math"

	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/system"
)

/*
PumpMoveOK is live volume-clock bid impulse from equation.Ignition — replaces
wall-clock L3 mark history for SessionMoveOK.
*/
func PumpMoveOK(pump *equation.IgnitionOutput) bool {
	if pump == nil {
		return false
	}

	sel := system.Cfg.Selection
	risk := system.Cfg.Risk

	if sel.IgnitionPrecursorMin > 0 && pump.Precursor < sel.IgnitionPrecursorMin {
		return false
	}

	if pump.RVOL <= 0 {
		return false
	}

	if risk.MinImpulseAccelRatio <= 0 {
		return false
	}

	pace := math.Max(pump.Ignition, pump.Trend)

	return pace >= risk.MinImpulseAccelRatio
}