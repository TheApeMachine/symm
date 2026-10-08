// Package distribution measures empirical distributions. Numeric operands and
// results cross the wire one scalar at a time. A pair of replayable point runs
// is carried by two core.Primitive values; each run yields position, weight,
// position, weight, ... . No numeric tuple is reinterpreted through a pointer.
package distribution

import (
	"iter"
	"math"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Normalize rescales the weight column of fixed-width scalar records to unit
// mass. Default layout is a run of weights; (2,1) is a point run. Other columns
// pass through unchanged. Empty/zero-mass input has no distribution.
type Normalize struct {
	*core.PrimitiveError
	width  int
	column int
}

func NewNormalize(layout ...int) core.Primitive {
	op := &Normalize{PrimitiveError: core.NewPrimitiveError(), width: 1}

	if len(layout) != 0 && len(layout) != 2 {
		op.Error(core.ErrShape)
		return op
	}

	if len(layout) == 2 {
		op.width, op.column = layout[0], layout[1]
	}

	if op.width <= 0 || op.column < 0 || op.column >= op.width {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		var values []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			values = append(values, *(*float64)(arriving))
		}

		if len(values)%op.width != 0 {
			op.Error(core.ErrShape)
			return
		}

		total := 0.0

		for index := op.column; index < len(values); index += op.width {
			if values[index] < 0 {
				op.Error(core.ErrDomain)
				return
			}

			total += values[index]
		}

		if total == 0 {
			return
		}

		for index := op.column; index < len(values); index += op.width {
			values[index] /= total
		}

		for pointer := range data.NewValue(values...).Next(nil) {
			if !yield(pointer) {
				return
			}
		}
	}
}

// SortedPositions orders a scalar point run by position without changing mass.
type SortedPositions struct{ *core.PrimitiveError }

func NewSortedPositions() core.Primitive {
	return &SortedPositions{PrimitiveError: core.NewPrimitiveError()}
}

func (op *SortedPositions) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			values = append(values, *(*float64)(arriving))
		}

		if len(values)%2 != 0 {
			op.Error(core.ErrShape)
			return
		}

		order := make([]int, len(values)/2)

		for index := range order {
			order[index] = index
		}

		sort.SliceStable(order, func(left, right int) bool {
			return values[2*order[left]] < values[2*order[right]]
		})

		for _, index := range order {
			for pointer := range data.NewValue(values[2*index], values[2*index+1]).Next(nil) {
				if !yield(pointer) {
					return
				}
			}
		}
	}
}

// MergedWalk compares two ascending point runs. It yields KS, Wasserstein-1,
// distinct-position count, in that order. Each side is normalized by its own
// mass. Equal positions are exhausted on both sides before comparing CDFs.
// Validation and totals use replay; the comparison is a single merged walk,
// without a union grid, support map, or resampling. No inputs means no comparison.
type MergedWalk struct{ *core.PrimitiveError }

func NewMergedWalk() core.Primitive {
	return &MergedWalk{PrimitiveError: core.NewPrimitiveError()}
}

func (op *MergedWalk) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var streams [2]core.Primitive
		count := 0

		for arriving := range in {
			if arriving == nil || count == len(streams) {
				op.Error(core.ErrShape)
				return
			}

			streams[count] = *(*core.Primitive)(arriving)
			count++
		}

		if count == 0 {
			return
		}

		if count != len(streams) || streams[0] == nil || streams[1] == nil {
			op.Error(core.ErrShape)
			return
		}

		var totals [2]float64

		for side, stream := range streams {
			index := 0
			previous := 0.0

			for arriving := range stream.Next(nil) {
				if arriving == nil {
					op.Error(core.ErrShape)
					return
				}

				value := *(*float64)(arriving)

				if index%2 == 0 {
					if index > 0 && value < previous {
						op.Error(core.ErrDomain)
						return
					}

					previous = value
				}

				if index%2 == 1 {
					if value < 0 {
						op.Error(core.ErrDomain)
						return
					}

					totals[side] += value
				}

				index++
			}

			if err := op.Error(stream.Error()); err != nil {
				return
			}

			if index%2 != 0 {
				op.Error(core.ErrShape)
				return
			}
		}

		if totals[0] == 0 || totals[1] == 0 {
			return
		}

		nextLeft, stopLeft := iter.Pull(streams[0].Next(nil))
		defer stopLeft()
		nextRight, stopRight := iter.Pull(streams[1].Next(nil))
		defer stopRight()
		next := [2]func() (unsafe.Pointer, bool){nextLeft, nextRight}
		var positions, weights, cumulative [2]float64
		var active [2]bool

		for side := range streams {
			pointer, ok := next[side]()
			active[side] = ok

			if ok {
				positions[side] = *(*float64)(pointer)
				pointer, ok = next[side]()

				if !ok || pointer == nil {
					op.Error(core.ErrShape)
					return
				}

				weights[side] = *(*float64)(pointer)
			}
		}

		previous, statistic, distance, distinct := 0.0, 0.0, 0.0, 0.0

		for active[0] || active[1] {
			position := positions[0]

			if !active[0] || (active[1] && positions[1] < position) {
				position = positions[1]
			}

			if distinct > 0 {
				distance += math.Abs(cumulative[0]-cumulative[1]) * (position - previous)
			}

			for side := range streams {
				for active[side] && positions[side] == position {
					cumulative[side] += weights[side] / totals[side]
					pointer, ok := next[side]()
					active[side] = ok

					if !ok {
						break
					}

					positions[side] = *(*float64)(pointer)
					pointer, ok = next[side]()

					if !ok || pointer == nil {
						op.Error(core.ErrShape)
						return
					}

					weights[side] = *(*float64)(pointer)
				}
			}

			statistic = math.Max(statistic, math.Abs(cumulative[0]-cumulative[1]))
			previous = position
			distinct++
		}

		if err := op.Error(streams[0].Error(), streams[1].Error()); err != nil {
			return
		}

		for pointer := range data.NewValue(statistic, distance, distinct).Next(nil) {
			if !yield(pointer) {
				return
			}
		}
	}
}

// Wasserstein1Pairs selects W1 from a merged walk of two point runs.
type Wasserstein1Pairs struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewWasserstein1Pairs() core.Primitive {
	return &Wasserstein1Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline:       nomagique.NewNumber(NewMergedWalk(), data.NewSelect(1)),
	}
}

func (op *Wasserstein1Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}

// KolmogorovSmirnovPairs selects KS from the same merged-walk definition.
type KolmogorovSmirnovPairs struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewKolmogorovSmirnovPairs() core.Primitive {
	return &KolmogorovSmirnovPairs{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline:       nomagique.NewNumber(NewMergedWalk(), data.NewSelect(0)),
	}
}

func (op *KolmogorovSmirnovPairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}

// Wasserstein1 accepts shared-support records: position, left mass, right mass.
type Wasserstein1 struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewWasserstein1() core.Primitive {
	return &Wasserstein1{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			transport.NewFanout[float64](
				nomagique.NewNumber(transport.NewMap(3, data.NewSelect(0, 1)), data.NewPack[float64]()),
				nomagique.NewNumber(transport.NewMap(3, data.NewSelect(0, 2)), data.NewPack[float64]()),
			),
			NewWasserstein1Pairs(),
		),
	}
}

func (op *Wasserstein1) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}

// KolmogorovSmirnov accepts the same shared-support scalar records.
type KolmogorovSmirnov struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewKolmogorovSmirnov() core.Primitive {
	return &KolmogorovSmirnov{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			transport.NewFanout[float64](
				nomagique.NewNumber(transport.NewMap(3, data.NewSelect(0, 1)), data.NewPack[float64]()),
				nomagique.NewNumber(transport.NewMap(3, data.NewSelect(0, 2)), data.NewPack[float64]()),
			),
			NewKolmogorovSmirnovPairs(),
		),
	}
}

func (op *KolmogorovSmirnov) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}

// Entropy measures Shannon entropy (nats) of a normalized scalar mass run.
type Entropy struct{ *core.PrimitiveError }

func NewEntropy() core.Primitive { return &Entropy{PrimitiveError: core.NewPrimitiveError()} }

func (op *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		entropy, count := 0.0, 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			weight := *(*float64)(arriving)

			if weight < 0 {
				op.Error(core.ErrDomain)
				return
			}

			if weight > 0 {
				entropy -= weight * math.Log(weight)
			}

			count++
		}

		if count > 0 {
			yield(unsafe.Pointer(&entropy))
		}
	}
}

// Concentration measures Herfindahl concentration of normalized scalar masses.
type Concentration struct{ *core.PrimitiveError }

func NewConcentration() core.Primitive {
	return &Concentration{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Concentration) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		concentration, count := 0.0, 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			weight := *(*float64)(arriving)

			if weight < 0 {
				op.Error(core.ErrDomain)
				return
			}

			concentration += weight * weight
			count++
		}

		if count > 0 {
			yield(unsafe.Pointer(&concentration))
		}
	}
}

// ConcentrationPoints composes point-weight extraction, normalization and HHI.
type ConcentrationPoints struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewConcentrationPoints() core.Primitive {
	return &ConcentrationPoints{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline:       nomagique.NewNumber(transport.NewMap(2, data.NewSelect(1)), NewNormalize(), NewConcentration()),
	}
}

func (op *ConcentrationPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}

// EntropyPoints composes the same extraction and normalization with entropy.
type EntropyPoints struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewEntropyPoints() core.Primitive {
	return &EntropyPoints{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline:       nomagique.NewNumber(transport.NewMap(2, data.NewSelect(1)), NewNormalize(), NewEntropy()),
	}
}

func (op *EntropyPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pipeline.Next(in) {
			if !yield(pointer) {
				return
			}
		}
		op.Error(op.pipeline.Error())
	}
}
