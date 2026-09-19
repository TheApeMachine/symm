package store

import (
	"math"

	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

type Grid[T, U any] func(message transport.Message[T, U]) []U

/*
NewGrid creates a lock-free primitive that acts as a "virtual" grid.
It does not actually create a 2D grid in memory. Instead, it uses a 1D slice
and assigns each cell a unique 2D coordinate when it registers.
A cell is just another primitive, wrapped in a bi-drectional Conn primitive.
Send a POKE message to the Grid with the raw data.
Send a PEEK message to the Grid to get all the cell values.
*/
func NewGrid[T, U any]() Grid[T, U] {
	cells := make([]transport.Conn[T, U], 0)
	coords := make([][2]int, 0)
	cellKeys := make([][]types.Value[T, *float64], 0)

	return func(message transport.Message[T, U]) []U {
		action, payload, primitive := message()

		switch action {
		case transport.REGISTER:
			if primitive != nil {
				// Allocate unique 2D coordinates dynamically using square expansion
				n := len(cells)
				s := int(math.Sqrt(float64(n)))
				rem := n - s*s
				x, y := 0, 0

				if rem < s {
					x, y = rem, s
				} else {
					x, y = s, rem-s
				}

				coords = append(coords, [2]int{x, y})

				// Extract keys from payload
				var keys []types.Value[T, *float64]
				if k, ok := any(payload).([]types.Value[T, *float64]); ok {
					keys = k
				}
				cellKeys = append(cellKeys, keys)

				var latest U

				rx := transport.NewIO(func(T) U { return latest })
				tx := transport.NewIO(func(in T) U {
					if res := primitive(in); any(res) != nil {
						latest = res
					}
					return latest
				})

				conn := transport.NewConn(rx, tx)
				cells = append(cells, conn)
			}

			return nil

		case transport.PEEK:
			// Call each metric with a PEEK message to get their values
			out := make([]U, len(cells))
			var zero T

			for i, cell := range cells {
				out[i] = cell(transport.PEEK, zero)
			}

			return out

		case transport.POKE:
			// Call each metric with a POKE message to pass them their requested data
			out := make([]U, len(cells))
			for i, cell := range cells {
				keys := cellKeys[i]

				if len(keys) > 0 {
					extracted := make([]*float64, len(keys))
					for j, key := range keys {
						extracted[j] = key(payload)
					}
					out[i] = cell(transport.POKE, any(extracted).(T))
				} else {
					out[i] = cell(transport.POKE, payload)
				}
			}
			return out
		}

		return nil
	}
}
