package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Identity constructs the diagonal in typed storage after Range validates the size. */
type Identity struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewIdentity enumerates the dimension once, without a scalar graph per coefficient. */
func NewIdentity() core.Primitive {
	size := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		size, transport.NewRange(size), transport.NewCollect[float64](),
		&Identity{seed: transport.NewIO(core.From([][]float64{}))},
	)
}

func (identity *Identity) Next(input core.Primitive) core.Primitive {
	result := core.Yield(identity.seed, input, func(_ [][]float64, indices []float64) [][]float64 {
		size := len(indices)
		rows := make([][]float64, size)
		values := make([]float64, size*size)

		for row := range rows {
			rows[row] = values[row*size : (row+1)*size]
			rows[row][row] = 1
		}
		return rows
	}, identity)

	if result != nil {
		identity.current = result
	}
	return result
}

func (identity *Identity) Read() any { return core.To[any](identity.current) }
