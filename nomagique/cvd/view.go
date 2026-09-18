package cvd

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type pick struct {
	*core.PrimitiveError
	selectField func(*Reading) float64
	out         float64
}

func (pick *pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*Reading)(arriving)
			pick.out = pick.selectField(reading)

			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}

func newPick(selectField func(*Reading) float64) *pick {
	return &pick{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewTradeCount() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.TradeCount })
}

func NewBuyCount() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.BuyCount })
}

func NewSellCount() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.SellCount })
}

func NewBuyQty() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.BuyQty })
}

func NewSellQty() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.SellQty })
}

func NewGrossQty() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.GrossQty })
}

func NewNetQty() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.NetQty })
}

func NewBuyNotional() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.BuyNotional })
}

func NewSellNotional() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.SellNotional })
}

func NewGrossNotional() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.GrossNotional })
}

func NewNetNotional() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.NetNotional })
}

func NewMeanNotional() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.MeanNotional })
}

func NewCVD() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.CVD })
}

func NewCND() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.CND })
}

func NewEpoch() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.Epoch })
}

func NewSignedCount() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.SignedCount })
}

func NewSignedNet() core.Primitive {
	return newPick(func(reading *Reading) float64 { return reading.SignedNet })
}
