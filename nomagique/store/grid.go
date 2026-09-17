package store

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"
)

/*
Grid is a coordinate-addressed store of distributed primitives. It owns their
addresses, not their values. Queries have one consumer; read outputs are
borrowed until the next query. Registered coordinates must not be mutated.
*/
type Grid[T interface {
	core.Ordered[T]
	comparable
}] struct {
	*core.PrimitiveError
	cells     lockfree.Map[T, core.Primitive]
	interests lockfree.Map[T, [][]string]
	count     int
	current   core.Primitive
}

/*
NewGrid registers each member at its existing identity within its entity.
*/
func NewGrid[T interface {
	core.Ordered[T]
	comparable
}](members ...core.Connectable[T]) *Grid[T] {
	grid := &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		cells: lf.NewSkipList[T, core.Primitive](
			func(left, right T) bool { return left.Less(right) },
		),
		interests: lf.NewSkipList[T, [][]string](
			func(left, right T) bool { return left.Less(right) },
		),
	}

	for _, primitive := range members {
		sequence.Read[*core.Query[T, core.Connectable[T]]](grid.Next(
			core.NewQuery[T, core.Connectable[T]](
				primitive, core.Identify,
			).Next(nil),
		))
	}

	return grid
}

/*
Next answers Query[T, core.Identifiable[T]]. Identify registers a single member and
uses its existing identity or allocates one when unassigned; Read yields the member;
Execute passes the borrowed payload through it. Missing cells, occupied addresses and
writes are errors.
*/
func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var query *core.Query[T, core.Connectable[T]]

		for arriving := range in {
			if query == nil {
				query = (*core.Query[T, core.Connectable[T]])(arriving)

				if query == nil || grid.Error() != nil {
					return
				}

				switch query.Action {
				case core.Identify:
					address := query.Identity()
					var zero T

					if any(address) == nil || any(address) == any(zero) {
						coord := any(geometry.NewCoordinate(grid.count, 0)).(T)
						grid.count++
						query.Identify(coord)
						address = coord
					}

					if _, occupied := grid.cells.Get(address); occupied {
						grid.Error(fmt.Errorf("%w: occupied address", core.ErrShape))
						return
					}

					publish := transport.NewAddress[T]()
					publish.Connect(transport.NewIO[any](nil, nil))

					query.Connect(publish)

					grid.cells.Set(address, query.Connectable)

					var endpoint core.Connectable[T] = publish

					if !yield(unsafe.Pointer(&endpoint)) {
						return
					}

					continue

				case core.Read:
					cell, found := grid.cells.Get(query.Identity())

					if !found {
						grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
						return
					}

					if !yield(unsafe.Pointer(&cell)) {
						return
					}

					query = nil
					continue

				case core.Write:
					grid.Error(fmt.Errorf("%w: grid writes are not allowed", core.ErrShape))
					return

				case core.Execute:
					continue
				}
			}

			if query.Action == core.Identify {
				interests := *(*[][]string)(arriving)

				if len(interests) > 0 {
					grid.interests.Set(query.Identity(), interests)
				}

				query = nil
				continue
			}

			if query.Action == core.Execute {
				cell, found := grid.cells.Get(query.Identity())

				if !found {
					grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
					return
				}

				single := func(y func(unsafe.Pointer) bool) { y(arriving) }

				for out := range cell.Next(single) {
					if !yield(out) {
						return
					}
				}
			}
		}
	}
}
