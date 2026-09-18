package sentiment

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type pick struct {
	*core.PrimitiveError
	selectField func(*Reading) (float64, bool)
	out         float64
}

func (pick *pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*Reading)(arriving)
			value, ok := pick.selectField(reading)

			if !ok {
				continue
			}

			pick.out = value

			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}

func newPick(selectField func(*Reading) (float64, bool)) *pick {
	return &pick{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewLast() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Last, true })
}

func NewValidCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Valid, reading.HasCohort })
}

func NewAdvanceCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Advance, reading.HasCohort })
}

func NewDeclineCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Decline, reading.HasCohort })
}

func NewUnchangedCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Unchanged, reading.HasCohort })
}

func NewBreadth() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Breadth, reading.HasCohort })
}

func NewMedianReturn() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Median, reading.HasCohort })
}

func NewMedianAbsolute() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.MedianAbs, reading.HasCohort })
}

func NewReturnMAD() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.MAD, reading.HasCohort })
}

func NewLargestAbsolute() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.LargestAbs, reading.HasCohort })
}

func NewSignedFraction() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SignedFrac, reading.HasCohort })
}

func NewSignedBaseline() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.SignedBase, reading.HasSignedBase
	})
}

func NewSignedZScore() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.SignedZ, reading.HasSignedBase
	})
}
