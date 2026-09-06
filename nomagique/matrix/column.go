package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewColumn arranges a scalar run as an n-by-one matrix.
func NewColumn() core.Primitive {
	return transport.NewPipe(transport.NewCollect[float64](), transport.NewCollect[[]float64](), NewTranspose[float64]())
}
