package adaptive

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewThreshold composes a moment estimator with a dispersion coefficient.
// Coefficient selection is a configured graph, not a ThresholdType switch.
func NewThreshold(moments, coefficient core.Primitive) core.Primitive {
	return transport.NewPipe(moments, transport.NewMap(equation.NewThreshold(coefficient)))
}
