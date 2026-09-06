package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewOuter lifts two vector-valued expressions to a column and a row.
func NewOuter(left, right core.Primitive) core.Primitive {
	return NewProduct(
		transport.NewPipe(left, transport.NewSpread[float64](), NewColumn()),
		transport.NewPipe(right, transport.NewCollect[[]float64]()),
	)
}
