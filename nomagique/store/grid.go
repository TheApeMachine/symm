package store

import (
	"cmp"
	"fmt"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

// Registration declares a writer, its exact input interests, and borrowed metric addresses.
// Query carries this declaration as the payload of ActionIdentify.
type Registration[T any] struct {
	Owner     string
	Operation core.Primitive
	Interests [][2]string
	Metrics   map[string]*T
}

/*
Grid indexes owner-resident metrics and routes borrowed flat input maps.
It never writes a metric. Values are addressed by (owner, metric); input
interests by (entity, raw key). Coordinates are independent mutable addresses,
not storage locations. Registration and execution have one writer, and owner
execution completes before the Grid is yielded to downstream computations.
*/
type Grid[T any] struct {
	*core.PrimitiveError
	Values      map[[2]string]*T
	Coordinates map[[2]string]*[2]float64
	owners      map[string]core.Primitive
	routes      map[[2]string][]int
	names       []string
	order       []int
	selected    []bool
	addresses   [][2]string
	writers     map[*T][2]string
	started     bool
}

func NewGrid[T any]() *Grid[T] {
	return &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		Values:         make(map[[2]string]*T),
		Coordinates:    make(map[[2]string]*[2]float64),
		owners:         make(map[string]core.Primitive),
		routes:         make(map[[2]string][]int),
		writers:        make(map[*T][2]string),
	}
}

/*
Next accepts the existing Query protocol. Identify binds an owner declaration;
Execute borrows each map[[2]string]T in the query payload and invokes each
matching owner once. Read yields this address index without executing owners.
Every answer is the same borrowed Grid, never a metric snapshot. Downstream
code must finish before the next input; concurrent access needs an external
execution barrier. An owner must neither retain nor mutate the raw input map.
*/
func (grid *Grid[T]) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			if grid.Error() != nil {
				return
			}

			query := (*Query[T])(arriving)
			switch query.Action() {
			case data.ActionIdentify:
				registration := sequence.Read[*Registration[T]](query.payload)

				if grid.started || registration == nil ||
					registration.Owner == "" || registration.Operation == nil ||
					len(registration.Interests) == 0 || len(registration.Metrics) == 0 {
					grid.Error(fmt.Errorf("%w: grid registration requires an owner, interests and resident metrics before execution", core.ErrShape))
					return
				}

				if grid.owners[registration.Owner] != nil {
					grid.Error(fmt.Errorf("%w: grid owner %q is already registered", core.ErrShape, registration.Owner))
					return
				}

				for _, interest := range registration.Interests {
					if interest[0] == "" || interest[1] == "" {
						grid.Error(fmt.Errorf("%w: grid interest requires an entity and raw key", core.ErrShape))
						return
					}
				}

				seen := make(map[*T]bool, len(registration.Metrics))
				for name, metric := range registration.Metrics {
					if name == "" || metric == nil || seen[metric] || grid.writers[metric] != [2]string{} {
						grid.Error(fmt.Errorf("%w: grid metric %q requires exclusive resident storage", core.ErrShape, name))
						return
					}
					seen[metric] = true
				}

				// Names define execution order, independently of registration order.
				identity := len(grid.names)
				grid.names = append(grid.names, registration.Owner)
				grid.order = append(grid.order, identity)
				grid.selected = append(grid.selected, false)
				grid.owners[registration.Owner] = registration.Operation
				slices.SortFunc(grid.order, func(left, right int) int {
					return cmp.Compare(grid.names[left], grid.names[right])
				})

				for _, interest := range registration.Interests {
					grid.routes[interest] = append(grid.routes[interest], identity)
				}

				for name, metric := range registration.Metrics {
					address := [2]string{registration.Owner, name}
					grid.Values[address] = metric
					grid.Coordinates[address] = new([2]float64)
					grid.writers[metric] = address
					grid.addresses = append(grid.addresses, address)
				}

				slices.SortFunc(grid.addresses, func(left, right [2]string) int {
					if ordering := cmp.Compare(left[0], right[0]); ordering != 0 {
						return ordering
					}
					return cmp.Compare(left[1], right[1])
				})

				// Canonical initial positions are ordinals; no metric occupies a cell.
				for index, address := range grid.addresses {
					*grid.Coordinates[address] = [2]float64{float64(index), 0}
				}
			case data.ActionExecute:
				if query.payload == nil {
					grid.Error(fmt.Errorf("%w: grid execution requires registered owners and an input stream", core.ErrShape))
					return
				}

				for raw := range query.payload {
					if len(grid.owners) == 0 {
						grid.Error(fmt.Errorf("%w: grid execution requires registered owners", core.ErrShape))
						return
					}

					grid.started = true
					clear(grid.selected)
					for address := range *(*map[[2]string]T)(raw) {
						for _, identity := range grid.routes[address] {
							grid.selected[identity] = true
						}
					}

					var delivery iter.Seq[unsafe.Pointer]
					for _, identity := range grid.order {
						if !grid.selected[identity] {
							continue
						}

						if delivery == nil {
							delivery = sequence.NewValue(*(*map[[2]string]T)(raw))
						}

						owner := grid.owners[grid.names[identity]]
						for range owner.Next(delivery) {
						}

						if err := owner.Error(); err != nil {
							grid.Error(fmt.Errorf("grid owner %q: %w", grid.names[identity], err))
							return
						}
					}

					if !yield(unsafe.Pointer(grid)) {
						return
					}
				}
				continue
			case data.ActionRead:
			default:
				grid.Error(fmt.Errorf("%w: grid writes belong to registered metric owners", core.ErrShape))
				return
			}

			if !yield(unsafe.Pointer(grid)) {
				return
			}
		}
	}
}
