package calculus

import "github.com/theapemachine/symm/nomagique"

var (
	// Structural ports. The legacy names remain aliases for source compatibility.
	PortA      = nomagique.MustIntern("left")
	PortB      = nomagique.MustIntern("right")
	PortX      = nomagique.MustIntern("value")
	PortResult = nomagique.MustIntern("result")

	SymbolLeft   = PortA
	SymbolRight  = PortB
	SymbolValue  = PortX
	SymbolResult = PortResult

	SymbolScale    = nomagique.MustIntern("scale")
	SymbolBaseline = nomagique.MustIntern("baseline")
	SymbolReady    = nomagique.MustIntern("ready")
	SymbolCurrent  = nomagique.MustIntern("current")
	SymbolPrevious = nomagique.MustIntern("previous")
	SymbolCount    = nomagique.MustIntern("count")
	SymbolDuration = nomagique.MustIntern("duration")
	SymbolRate     = nomagique.MustIntern("rate")
	SymbolBase     = nomagique.MustIntern("base")
	SymbolJump     = nomagique.MustIntern("jump")
	SymbolLevel    = nomagique.MustIntern("level")
	SymbolClock    = nomagique.MustIntern("clock")
	SymbolShape    = nomagique.MustIntern("shape")
	SymbolProgress = nomagique.MustIntern("progress")
	SymbolTotal    = nomagique.MustIntern("total")
	SymbolDelta    = nomagique.MustIntern("delta")
)
