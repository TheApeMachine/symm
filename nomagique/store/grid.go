package store

import (
	"context"
	"fmt"
	"iter"
	"math"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree/lf"
	"golang.org/x/sync/errgroup"
)

/*
Grid is a coordinate-addressed store of distributed primitives. Coordinates
are keys to cells. Queries have one consumer; read outputs are
borrowed until the next query. Registered coordinates must not be mutated.
*/
type Grid[T interface {
	core.Ordered[T]
	comparable
}] struct {
	*core.PrimitiveError
	cells      *lf.SkipList[T, core.Primitive]
	interests  *lf.SkipList[T, [][]string]
	count      atomic.Int64
	current    core.Primitive
	writeQueue *lf.Queue[[]unsafe.Pointer]
	writing    atomic.Int32
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
		writeQueue: lf.NewQueue[[]unsafe.Pointer](),
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
Next answers Query. Identify registers a member; Read yields retained
observations; Write routes each keyed market payload to cells whose
registered interests match; Execute with an identity drives that cell.
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
					coord := query.Identity()

					if any(coord) == nil || any(coord) == any(zero) {
						generated := any(geometry.NewCoordinate(int(grid.count.Add(1)-1), 0)).(T)
						query.Identify(generated)
						coord = generated
					}

					if _, occupied := grid.cells.Get(coord); occupied {
						grid.Error(fmt.Errorf("%w: occupied coordinate", core.ErrShape))
						return
					}

					publish := transport.NewAddress[T]()
					publish.Connect(transport.NewIO[any](nil, nil))

					query.Connect(publish)

					grid.cells.Set(coord, query.Connectable)
					endpoint = publish
					identified = true
					continue

				case core.Read:
					coord := query.Identity()

					if any(coord) == nil || any(coord) == any(zero) {
						group, groupCtx := errgroup.WithContext(context.Background())
						readingsChan := make(chan *core.Input[T, string, float64], grid.cells.Len()+1)

						from := any(geometry.NewCoordinate(math.MinInt, math.MinInt)).(T)
						to := any(geometry.NewCoordinate(math.MaxInt, math.MaxInt)).(T)

						grid.cells.Range(from, to, func(targetCoord T, cell core.Primitive) {
							targetCell := cell

							group.Go(func() error {
								if groupCtx.Err() != nil {
									return groupCtx.Err()
								}

								for out := range targetCell.Next(nil) {
									val := new(float64)
									*val = *(*float64)(out)
									origin, _ := targetCell.(core.Connectable[T])
									reading := core.NewInput(origin, core.Read, "", val)

									select {
									case readingsChan <- reading:
									case <-groupCtx.Done():
										return groupCtx.Err()
									}
								}

								return nil
							})
						})

						go func() {
							_ = group.Wait()
							close(readingsChan)
						}()

						var collected []*core.Input[T, string, float64]

						for reading := range readingsChan {
							collected = append(collected, reading)
						}

						slices.SortFunc(collected, func(left, right *core.Input[T, string, float64]) int {
							if left.Origin.Identity().Less(right.Origin.Identity()) {
								return -1
							}

							if right.Origin.Identity().Less(left.Origin.Identity()) {
								return 1
							}

							return 0
						})

						for _, reading := range collected {
							if !yield(unsafe.Pointer(reading)) {
								return
							}
						}

						query = nil
						continue
					}

					cell, found := grid.cells.Get(coord)

					if !found {
						grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
						return
					}

					for out := range cell.Next(nil) {
						held := new(float64)
						*held = *(*float64)(out)
						origin, _ := cell.(core.Connectable[T])
						reading := core.NewInput(
							origin, core.Read, "", held,
						)

						if !yield(unsafe.Pointer(reading)) {
							return
						}
					}

					query = nil
					continue

				case core.Write:
					continue

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

			if query.Action == core.Write {
				event = append(event, arriving)
				continue
			}

			if query.Action == core.Execute {
				coord := query.Identity()

				if any(coord) == nil || any(coord) == any(zero) {
					event = append(event, arriving)
					continue
				}

				cell, found := grid.cells.Get(coord)

				if !found {
					grid.Error(fmt.Errorf("%w: missing cell", core.ErrShape))
					return
				}

				single := func(singleYield func(unsafe.Pointer) bool) { singleYield(arriving) }

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
			coord := query.Identity()

			if any(coord) != nil && any(coord) != any(zero) && len(event) == 0 {
				cell, found := grid.cells.Get(coord)

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

		if query == nil || len(event) == 0 {
			return
		}

		if query.Action != core.Write && query.Action != core.Execute {
			return
		}

		coord := query.Identity()

		if query.Action == core.Execute && any(coord) != nil && any(coord) != any(zero) {
			return
		}

		grid.writeQueue.Enqueue(event)

		if !grid.writing.CompareAndSwap(0, 1) {
			return
		}

		for {
			batch, ok := grid.writeQueue.Dequeue()

			if !ok {
				grid.writing.Store(0)

				if grid.writeQueue.Length() > 0 && grid.writing.CompareAndSwap(0, 1) {
					continue
				}

				return
			}

			for {
				nextBatch, hasNext := grid.writeQueue.Dequeue()

				if !hasNext {
					break
				}

				batch = append(batch, nextBatch...)
			}

			group, groupCtx := errgroup.WithContext(context.Background())
			from := any(geometry.NewCoordinate(math.MinInt, math.MinInt)).(T)
			to := any(geometry.NewCoordinate(math.MaxInt, math.MaxInt)).(T)

			grid.cells.Range(from, to, func(targetCoord T, cell core.Primitive) {
				wanted, haveInterests := grid.interests.Get(targetCoord)

				if !haveInterests || len(wanted) == 0 {
					return
				}

				targetCell := cell
				cellInterests := wanted

				group.Go(func() error {
					if groupCtx.Err() != nil {
						return groupCtx.Err()
					}

					var matched []unsafe.Pointer

					for _, payload := range batch {
						input := (*core.Input[string, []string, any])(payload)

						if input == nil {
							continue
						}

						for _, interest := range cellInterests {
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
						return nil
					}

					run := func(yieldRun func(unsafe.Pointer) bool) {
						for _, payload := range matched {
							if !yieldRun(payload) {
								return
							}
						}
					}

					for range targetCell.Next(run) {
					}

					return nil
				})
			})

			if err := group.Wait(); err != nil {
				grid.Error(err)
			}
		}
	}
}
