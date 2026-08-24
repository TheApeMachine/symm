package calculus

import (
	types "github.com/theapemachine/symm/nomagique/types"
)

var (
	// Structural ports. The legacy names remain aliases for source compatibility.
	PortA      = types.PortA
	PortB      = types.PortB
	PortX      = types.PortX
	PortResult = types.PortResult

	SymbolLeft   = PortA
	SymbolRight  = PortB
	SymbolValue  = PortX
	SymbolResult = PortResult

	SymbolScale    = types.MustIntern("scale")
	SymbolBaseline = types.MustIntern("baseline")
	SymbolReady    = types.MustIntern("ready")
	SymbolCurrent  = types.MustIntern("current")
	SymbolPrevious = types.MustIntern("previous")
	SymbolCount    = types.MustIntern("count")
	SymbolDuration = types.MustIntern("duration")
	SymbolRate     = types.MustIntern("rate")
	SymbolBase     = types.MustIntern("base")
	SymbolJump     = types.MustIntern("jump")
	SymbolLevel    = types.MustIntern("level")
	SymbolClock    = types.MustIntern("clock")
	SymbolShape    = types.MustIntern("shape")
	SymbolProgress = types.MustIntern("progress")
	SymbolTotal    = types.MustIntern("total")
	SymbolDelta    = types.MustIntern("delta")
)
