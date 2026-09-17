package store

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"
)

/*
Grid is a coordinate-addressed store of distributed primitives. It owns their
addresses, not their values. Queries have one consumer; read outputs are
borrowed until the next query. Registered coordinates must not be mutated.
*/
type Grid[T core.Ordered[T]] struct {
	*core.PrimitiveError
	cells   lockfree.Map[string, lockfree.Map[T, core.Primitive]]
	current core.Primitive
}

/*
NewGrid registers each member at its existing identity within its entity.
*/
func NewGrid[T core.Ordered[T]](members ...map[string]core.Identifiable[T]) *Grid[T] {
	grid := &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		cells: lf.NewSkipList[string, lockfree.Map[T, core.Primitive]](
			func(left, right string) bool { return left < right },
		),
	}

	for _, member := range members {
		for entity, primitive := range member {
			query := NewQuery[T, core.Identifiable[T]](
				primitive, data.ActionIdentify, sequence.NewValue(primitive),
			)

			query.Entity = entity

			for range grid.Next(query.Next(nil)) {
			}
		}
	}

	return grid
}

/*
Next answers Query[T, core.Identifiable[T]]. Identify registers a single member and
uses its existing identity; Read yields the member; Execute passes the borrowed
payload through it. Missing cells, occupied addresses and writes are errors.
*/
func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if grid.Error() != nil {
				return
			}

			query := (*Query[T, core.Identifiable[T]])(arriving)
			address := query.Identity()

			if query.Entity == "" {
				grid.Error(fmt.Errorf(
					"%w: grid query needs an entity", core.ErrShape,
				))

				return
			}

			entity, exists := grid.cells.Get(query.Entity)

			if query.Action() == data.ActionIdentify {
				member := query.First()

				if member == nil {
					grid.Error(fmt.Errorf(
						"%w: grid registration requires a member", core.ErrWrongType,
					))

					return
				}

				if !exists {
					entity = lf.NewSkipList[T, core.Primitive](T.Less)

					grid.cells.Set(query.Entity, entity)
				}

				if _, occupied := entity.Get(address); occupied || address.Less(member.Identity()) || member.Identity().Less(address) {
					grid.Error(fmt.Errorf(
						"%w: grid address is occupied or differs from the member identity", core.ErrShape,
					))

					return
				}

				grid.current = member
				entity.Set(address, grid.current)
			}

			if !exists && query.Action() != data.ActionIdentify {
				grid.Error(fmt.Errorf(
					"%w: grid entity %q", core.ErrNotHeld, query.Entity,
				))

				return
			}

			grid.current, exists = entity.Get(address)

			if !exists {
				grid.Error(fmt.Errorf(
					"%w: grid address %v", core.ErrNotHeld, address,
				))

				return
			}

			switch query.Action() {
			case data.ActionIdentify, data.ActionRead:
				if !yield(unsafe.Pointer(&grid.current)) {
					return
				}
			case data.ActionExecute:
				member := grid.current

				for output := range member.Next(query.payload) {
					if !yield(output) {
						grid.Error(member.Error())
						return
					}
				}
				grid.Error(member.Error())
			default:
				grid.Error(fmt.Errorf(
					"%w: grid writes belong to registered members", core.ErrDomain,
				))

				return
			}
		}
	}
}
