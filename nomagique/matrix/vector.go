package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewVector multiplies a matrix expression by a vector expression, retaining
// the vector result rather than a one-column matrix payload.
func NewVector(matrix, vector core.Primitive) core.Primitive {
	return transport.NewPipe(
		NewProduct(matrix, transport.NewPipe(vector, transport.NewSpread[float64](), NewColumn())),
		transport.NewSpread[[]float64](),
		transport.NewSpread[float64](),
		transport.NewCollect[float64](),
	)
}
