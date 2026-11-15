package strategy

import (
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/system"
)

/*
ignitionHot is true when the composite ignition reading clears its floor and at
least one of precursor/compression agrees. OR-only gates lit flat-liners on a
single precursor flicker and unlocked the wider peak cap.
*/
func ignitionHot(reading *equation.IgnitionOutput) bool {
	if reading == nil {
		return false
	}

	sel := system.Cfg.Selection

	if reading.Ignition < sel.IgnitionIgnitionMin {
		return false
	}

	return reading.Precursor >= sel.IgnitionPrecursorMin ||
		reading.Compression >= sel.IgnitionCompressionMin
}
