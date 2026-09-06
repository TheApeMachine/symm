package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewAugment joins corresponding rows horizontally. Zip rejects unequal row
// counts, and the transpose round trips reject ragged input matrices.
func NewAugment(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(
			transport.NewPipe(left, NewTranspose[float64](), NewTranspose[float64](), transport.NewSpread[[]float64]()),
			transport.NewPipe(right, NewTranspose[float64](), NewTranspose[float64](), transport.NewSpread[[]float64]()),
		),
		transport.NewMap(
			transport.NewPipe(transport.NewSpread[core.Primitive](), transport.NewSpread[float64](), transport.NewCollect[float64]()),
		),
		transport.NewCollect[[]float64](),
	)
}
