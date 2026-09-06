package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewFinite checks matrix entries through the shared scalar predicate.
func NewFinite() core.Primitive {
	return transport.NewPipe(transport.NewSpread[[]float64](), transport.NewSpread[float64](), vector.NewFinite())
}
