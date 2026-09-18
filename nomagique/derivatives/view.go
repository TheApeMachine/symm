package derivatives

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type pickBasis struct {
	*core.PrimitiveError
	selectField func(*BasisReading) (float64, bool)
	out         float64
}

func (pick *pickBasis) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*BasisReading)(arriving)
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

func newBasisPick(selectField func(*BasisReading) (float64, bool)) *pickBasis {
	return &pickBasis{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewDerivativePrice() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) { return reading.Last, true })
}

func NewReferencePrice() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) { return reading.Index, true })
}

func NewOpenInterest() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) { return reading.OI, true })
}

func NewBasisValue() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) { return reading.Basis, true })
}

func NewLogBasis() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) { return reading.LogBasis, reading.HasLog })
}

func NewBasisBaseline() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) {
		return reading.BasisBase, reading.HasBasisBase
	})
}

func NewBasisZScore() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) {
		return reading.BasisZ, reading.HasBasisBase
	})
}

func NewOIChange() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) {
		return reading.OIChange, reading.HasOIChange
	})
}

func NewOIGrowth() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) {
		return reading.OIGrowth, reading.HasOIGrowth
	})
}

func NewReturnGap() core.Primitive {
	return newBasisPick(func(reading *BasisReading) (float64, bool) {
		return reading.ReturnGap, reading.HasReturns
	})
}

type pickLiq struct {
	*core.PrimitiveError
	selectField func(*LiquidationReading) (float64, bool)
	out         float64
}

func (pick *pickLiq) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*LiquidationReading)(arriving)
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

func newLiqPick(selectField func(*LiquidationReading) (float64, bool)) *pickLiq {
	return &pickLiq{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewGrossTradeNotional() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) { return reading.TradeNotional, true })
}

func NewLiquidationBuy() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) { return reading.Buy, true })
}

func NewLiquidationSell() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) { return reading.Sell, true })
}

func NewGrossLiquidation() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) { return reading.Gross, true })
}

func NewNetLiquidation() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) { return reading.Net, true })
}

func NewLiquidationShare() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) {
		return reading.Share, reading.HasShare
	})
}

func NewLiquidationSigned() core.Primitive {
	return newLiqPick(func(reading *LiquidationReading) (float64, bool) {
		return reading.Signed, reading.HasSigned
	})
}
