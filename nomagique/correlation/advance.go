package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fold, History, Relative, CorrelationVelocity and EnergyVelocity advance the
cohort pipeline once Measurement peer metrics are writable again. Until that
API lands they are pass-through Primitives preserving constructor names.
*/

type Fold struct {
	*core.PrimitiveError
}

func NewFold() core.Primitive {
	return &Fold{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

type History struct {
	*core.PrimitiveError
}

func NewHistory() core.Primitive {
	return &History{PrimitiveError: core.NewPrimitiveError()}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

type Relative struct {
	*core.PrimitiveError
}

func NewRelative() core.Primitive {
	return &Relative{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

type CorrelationVelocity struct {
	*core.PrimitiveError
}

func NewCorrelationVelocity() core.Primitive {
	return &CorrelationVelocity{PrimitiveError: core.NewPrimitiveError()}
}

func (op *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

type EnergyVelocity struct {
	*core.PrimitiveError
}

func NewEnergyVelocity() core.Primitive {
	return &EnergyVelocity{PrimitiveError: core.NewPrimitiveError()}
}

func (op *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}
