package morphology

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

func NewDistance() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Distance, true })
}

func NewKS() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.KS, true })
}

func NewBidConcentration() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.ConcBid, true })
}

func NewAskConcentration() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.ConcAsk, true })
}

func NewBidEntropy() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.EntBid, true })
}

func NewAskEntropy() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.EntAsk, true })
}

func NewChange() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Change, reading.HasChange })
}
