package depthflow

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

func NewBidNotional() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.BidNotional, true })
}

func NewAskNotional() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AskNotional, true })
}

func NewBookNotional() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Notional, true })
}

func NewBookImbalance() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.Imbalance, reading.HasImbalance
	})
}

func NewAddedBid() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AddedBid, reading.HasFlow })
}

func NewAddedAsk() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.AddedAsk, reading.HasFlow })
}

func NewRemovedBid() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.RemovedBid, reading.HasFlow })
}

func NewRemovedAsk() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.RemovedAsk, reading.HasFlow })
}

func NewTurnover() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) { return reading.Turnover, reading.HasFlow })
}

func NewImbalanceBaseline() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.ImbalanceBase, reading.HasImbalanceBase
	})
}

func NewImbalanceZScore() core.Primitive {
	return newPick(func(reading *Reading) (float64, bool) {
		return reading.ImbalanceZ, reading.HasImbalanceBase
	})
}
