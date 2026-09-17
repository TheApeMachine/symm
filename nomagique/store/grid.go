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
	addresses []T
	count     int
	current   core.Primitive
	reading   core.Input[T, string, float64]
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
uses its existing identity or allocates one when unassigned; Read yields each
cell's retained observation without recomputing; Execute with an identity passes
the borrowed payload through that cell; Execute without an identity routes
keyed market inputs to cells whose registered interests match. Missing cells,
occupied addresses and writes are errors.
*/
func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var query *core.Query[T, core.Connectable[T]]
		var event []unsafe.Pointer
		var zero T
		var endpoint core.Connectable[T]
		identified := false

		for arriving := range in {
			if query == nil {
				query = (*core.Query[T, core.Connectable[T]])(arriving)

				if query == nil || grid.Error() != nil {
					return
				}

				switch query.Action {
				case core.Identify:
					address := query.Identity()

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
					grid.addresses = append(grid.addresses, address)
					endpoint = publish
					identified = true
					continue

				case core.Read:
					address := query.Identity()

					if any(address) == nil || any(address) == any(zero) {
						for _, held := range grid.addresses {
							cell, found := grid.cells.Get(held)

							if !found {
								continue
							}

							for out := range cell.Next(nil) {
								if !grid.yieldSlot(cell, out, yield) {
									return
								}
							}
						}

						query = nil
						continue
					}

					cell, found := grid.cells.Get(address)

					if !found {
						grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
						return
					}

					for out := range cell.Next(nil) {
						if !grid.yieldSlot(cell, out, yield) {
							return
						}
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
				address := query.Identity()

				if any(address) == nil || any(address) == any(zero) {
					event = append(event, arriving)
					continue
				}

				cell, found := grid.cells.Get(address)

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

		if identified && endpoint != nil {
			if !yield(unsafe.Pointer(&endpoint)) {
				return
			}
		}

		if query != nil && query.Action == core.Execute {
			address := query.Identity()

			if any(address) != nil && any(address) != any(zero) && len(event) == 0 {
				cell, found := grid.cells.Get(address)

				if !found {
					grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
					return
				}

				for out := range cell.Next(nil) {
					if !yield(out) {
						return
					}
				}

				return
			}
		}

		if query == nil || query.Action != core.Execute || len(event) == 0 {
			return
		}

		address := query.Identity()

		if any(address) != nil && any(address) != any(zero) {
			return
		}

		for _, held := range grid.addresses {
			wanted, haveInterests := grid.interests.Get(held)

			if !haveInterests || len(wanted) == 0 {
				continue
			}

			matched := make([]unsafe.Pointer, 0, len(event))

			for _, payload := range event {
				input := (*core.Input[string, []string, any])(payload)

				if input == nil {
					continue
				}

				for _, interest := range wanted {
					if len(interest) != len(input.Key) {
						continue
					}

					same := true

					for index := range interest {
						if interest[index] != input.Key[index] {
							same = false
							break
						}
					}

					if !same {
						continue
					}

					matched = append(matched, payload)
					break
				}
			}

			if len(matched) == 0 {
				continue
			}

			cell, found := grid.cells.Get(held)

			if !found {
				continue
			}

			run := func(y func(unsafe.Pointer) bool) {
				for _, payload := range matched {
					if !y(payload) {
						return
					}
				}
			}

			for range cell.Next(run) {
			}
		}
	}
}

func (grid *Grid[T]) yieldSlot(
	cell core.Primitive, out unsafe.Pointer, yield func(unsafe.Pointer) bool,
) bool {
	slot := *(*Slot[float64])(out)
	held := new(float64)
	*held = slot.Value
	origin, _ := cell.(core.Connectable[T])
	grid.reading = *core.NewInput(
		origin, core.Read, slot.Key, held,
	)

	return yield(unsafe.Pointer(&grid.reading))
}
