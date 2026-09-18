package hawkes

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

func NewEventCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.EventCount, true })
}

func NewBuyCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BuyCount, true })
}

func NewSellCount() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SellCount, true })
}

func NewBuyFraction() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BuyFraction, true })
}

func NewSellFraction() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SellFraction, true })
}

func NewArrivalRate() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.ArrivalRate, reading.HasRates
	})
}

func NewBuyRate() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.BuyRate, reading.HasRates
	})
}

func NewSellRate() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.SellRate, reading.HasRates
	})
}

func NewConditionalIntensity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.Lambda, reading.HasFit
	})
}

func NewBuyIntensity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.LambdaBuy, reading.HasFit
	})
}

func NewSellIntensity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.LambdaSell, reading.HasFit
	})
}

func NewSpectralRadius() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.SpectralRadius, reading.HasFit
	})
}
