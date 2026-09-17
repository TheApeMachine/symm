package liquidity

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

func NewBid() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Bid, true })
}

func NewAsk() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Ask, true })
}

func NewBidQty() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidQty, true })
}

func NewAskQty() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskQty, true })
}

func NewMidpoint() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Midpoint, true })
}

func NewSpread() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Spread, true })
}

func NewRelativeSpread() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Relative, true })
}

func NewBidNotional() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidNotional, true })
}

func NewAskNotional() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskNotional, true })
}

func NewTwoSided() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.TwoSided, true })
}

func NewImbalance() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Imbalance, true })
}

func NewBidBaseline() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidBaseline, reading.HasBaseline })
}

func NewAskBaseline() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskBaseline, reading.HasBaseline })
}

func NewSpreadBaseline() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadBaseline, reading.HasBaseline })
}

func NewBidRatio() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidRatio, reading.HasBaseline })
}

func NewAskRatio() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskRatio, reading.HasBaseline })
}

func NewSpreadRatio() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadRatio, reading.HasBaseline })
}

func NewBidDivergence() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidDivergence, reading.HasBaseline })
}

func NewAskDivergence() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskDivergence, reading.HasBaseline })
}

func NewSpreadDivergence() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadDiv, reading.HasBaseline })
}

func NewBidZ() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidZ, reading.HasBaseline })
}

func NewAskZ() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskZ, reading.HasBaseline })
}

func NewSpreadZ() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadZ, reading.HasBaseline })
}

func NewBidNoise() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidNoise, reading.HasBaseline })
}

func NewAskNoise() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskNoise, reading.HasBaseline })
}

func NewSpreadNoise() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadNoise, reading.HasBaseline })
}

func NewBidVelocity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidVelocity, reading.HasBaseline })
}

func NewAskVelocity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskVelocity, reading.HasBaseline })
}

func NewSpreadVelocity() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadVelocity, reading.HasBaseline })
}

func NewBidVelSNR() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidVelSNR, reading.HasBaseline })
}

func NewAskVelSNR() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskVelSNR, reading.HasBaseline })
}

func NewSpreadVelSNR() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.SpreadVelSNR, reading.HasBaseline })
}
