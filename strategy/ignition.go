package strategy

import (
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/system"
)

/*
ignitionHot is true when pumpdump labels cross configured ignition floors.
*/
func ignitionHot(reading *equation.IgnitionOutput) bool {
	if reading == nil {
		return false
	}

	sel := system.Cfg.Selection

	return reading.Precursor >= sel.IgnitionPrecursorMin ||
		reading.Compression >= sel.IgnitionCompressionMin ||
		reading.Ignition >= sel.IgnitionIgnitionMin
}
