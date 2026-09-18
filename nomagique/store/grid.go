package store

import (
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree/lf"
)

/*
Grid is an addressed store of distributed primitives.
Queries have one consumer; read outputs are borrowed until the next query.
*/
type Grid[T comparable] struct {
	*core.PrimitiveError
	cells     *lf.SkipList[T, core.Primitive]
	interests *lf.SkipList[T, [][]string]
	count     atomic.Int64
}

/*
NewGrid registers each member at its existing identity within its entity.
*/
func NewGrid[T comparable](members ...core.Connectable[T]) *Grid[T] {
	less := func(left, right T) bool {
		if ordered, ok := any(left).(core.Ordered[T]); ok {
			return ordered.Less(right)
		}

		if leftStr, ok := any(left).(string); ok {
			return leftStr < any(right).(string)
		}

		return false
	}

	grid := &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		cells:          lf.NewSkipList[T, core.Primitive](less),
		interests:      lf.NewSkipList[T, [][]string](less),
	}

	for _, member := range members {
		sequence.Read[core.Connectable[T]](grid.Next(
			core.NewQuery[T, core.Connectable[T]](
				member, core.Identify,
			).Next(nil),
		))
	}

	return grid
}

func bounds[T comparable]() (T, T) {
	var zero T

	if _, ok := any(zero).(string); ok {
		return any("").(T), any("\xff").(T)
	}

	return any(geometry.NewCoordinate(math.MinInt, math.MinInt)).(T),
		any(geometry.NewCoordinate(math.MaxInt, math.MaxInt)).(T)
}

func generateIdentity[T comparable](count int) T {
	var zero T

	if _, ok := any(zero).(string); ok {
		return any(strconv.Itoa(count)).(T)
	}

	return any(geometry.NewCoordinate(count, 0)).(T)
}

/*
Next answers Query. Identify registers a member; Read yields retained
observations; Write routes each keyed market payload to cells whose
registered interests match; Execute with an identity drives that cell.
*/
func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil || grid.Error() != nil {
			return
		}

		from, to := bounds[T]()

		for stream := range in {
			query := (*core.Query[T, core.Connectable[T]])(stream)

			if query == nil {
				return
			}

			switch query.Action {
			case core.Identify:
				var zero T
				coord := query.Identity()

				if any(coord) == nil || any(coord) == any(zero) {
					coord = generateIdentity[T](int(grid.count.Add(1) - 1))
					query.Identify(coord)
				}

				addr := transport.NewAddress[T]()
				addr.Identify(coord)
				query.Connect(addr)

				grid.cells.Set(coord, query.Connectable)

				if query.Payload != nil {
					for ptr := range query.Payload {
						interests := *(*[][]string)(ptr)
						grid.interests.Set(coord, interests)
					}
				}

				var connectable core.Connectable[T] = addr

				if !yield(unsafe.Pointer(&connectable)) {
					return
				}

			case core.Read:
				grid.cells.Range(from, to, func(key T, value core.Primitive) {
					for out := range value.Next(nil) {
						if !yield(out) {
							return
						}
					}
				})
			case core.Write, core.Execute:
				grid.cells.Range(from, to, func(cellKey T, cellValue core.Primitive) {
					interests, found := grid.interests.Get(cellKey)

					if !found {
						grid.Error(errnie.Err(
							errnie.NotFound,
							fmt.Sprintf("[grid] no interests found for key %v", cellKey),
							nil,
						))

						return
					}

					if query.Payload == nil {
						return
					}

					for payload := range query.Payload {
						var mapped map[string]any
						var rawString string
						isString := false

						switch valueType := (*(*any)(payload)).(type) {
						case map[string]any:
							mapped = valueType

						case string:
							rawString = valueType
							isString = true
						}

						if !isString && mapped == nil {
							continue
						}

						for _, interest := range interests {
							var extracted any

							if isString {
								if len(interest) == 0 || interest[0] != rawString {
									continue
								}

								extracted = rawString
							}

							if !isString && mapped != nil {
								current := any(mapped)

								for _, segment := range interest {
									if nestedMap, isMap := current.(map[string]any); isMap {
										current = nestedMap[segment]
										continue
									}

									if slice, isSlice := current.([]any); isSlice && len(slice) > 0 {
										if firstMap, isMap := slice[0].(map[string]any); isMap {
											current = firstMap[segment]
											continue
										}
									}

									if slice, isSlice := current.([]map[string]any); isSlice && len(slice) > 0 {
										current = slice[0][segment]
										continue
									}

									current = nil
									break
								}

								if current == nil {
									continue
								}

								lastSegment := ""
								if len(interest) > 0 {
									lastSegment = interest[len(interest)-1]
								}

								extracted = current

								if lastSegment == "timestamp" {
									switch val := current.(type) {
									case int64:
										extracted = val
									case float64:
										extracted = int64(val)
									case json.Number:
										if i, err := val.Int64(); err == nil {
											extracted = i
										}

										if _, err := val.Int64(); err != nil {
											if f, err := val.Float64(); err == nil {
												extracted = int64(f)
											}
										}
									case string:
										if parsedTime, err := time.Parse(time.RFC3339Nano, val); err == nil {
											extracted = parsedTime.UnixNano()
										}

										if parsedTime, err := time.Parse(time.RFC3339, val); err == nil {
											extracted = parsedTime.UnixNano()
										}

										if i, err := strconv.ParseInt(val, 10, 64); err == nil {
											extracted = i
										}
									}
								}

								if lastSegment != "timestamp" {
									switch val := current.(type) {
									case json.Number:
										if f, err := val.Float64(); err == nil {
											extracted = f
										}

										if _, err := val.Float64(); err != nil {
											extracted = val.String()
										}
									case string:
										if lastSegment != "symbol" && lastSegment != "side" && lastSegment != "type" {
											if f, err := strconv.ParseFloat(val, 64); err == nil {
												extracted = f
											}
										}
									}
								}
							}

							input := core.NewInput[any](
								nil,
								core.Write,
								interest,
								&extracted,
							)

							for result := range cellValue.Next(input.Next(nil)) {
								if !yield(result) {
									return
								}
							}
						}
					}
				})
			}
		}
	}
}
