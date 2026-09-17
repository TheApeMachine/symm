package toxicity

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type pickDisposition struct {
	*core.PrimitiveError
	selectField func(*DispositionReading) (float64, bool)
	out         float64
}

func (pick *pickDisposition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*DispositionReading)(arriving)
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

func newDispositionPick(selectField func(*DispositionReading) (float64, bool)) *pickDisposition {
	return &pickDisposition{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewBid() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) { return reading.Bid, true })
}

func NewAsk() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) { return reading.Ask, true })
}

func NewBidQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) { return reading.BidQty, true })
}

func NewAskQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) { return reading.AskQty, true })
}

func NewBidLogChange() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.BidLogChange, reading.HasBidLog
	})
}

func NewAskLogChange() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.AskLogChange, reading.HasAskLog
	})
}

func NewRetreatedBidQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.RetreatedBidQty, reading.HasPrev
	})
}

func NewRetreatedAskQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.RetreatedAskQty, reading.HasPrev
	})
}

func NewWithdrawnBidQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.WithdrawnBidQty, reading.HasPrev
	})
}

func NewWithdrawnAskQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.WithdrawnAskQty, reading.HasPrev
	})
}

func NewReplenishedBidQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.ReplenishedBidQty, reading.HasPrev
	})
}

func NewReplenishedAskQty() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.ReplenishedAskQty, reading.HasPrev
	})
}

func NewRetreatBidFraction() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.RetreatBidFrac, reading.HasPrev
	})
}

func NewRetreatAskFraction() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.RetreatAskFrac, reading.HasPrev
	})
}

func NewWithdrawBidFraction() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.WithdrawBidFrac, reading.HasPrev
	})
}

func NewWithdrawAskFraction() core.Primitive {
	return newDispositionPick(func(reading *DispositionReading) (float64, bool) {
		return reading.WithdrawAskFrac, reading.HasPrev
	})
}

type pickMatch struct {
	*core.PrimitiveError
	selectField func(*MatchReading) (float64, bool)
	out         float64
}

func (pick *pickMatch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*MatchReading)(arriving)
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

func newMatchPick(selectField func(*MatchReading) (float64, bool)) *pickMatch {
	return &pickMatch{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewTradeQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.Qty, true })
}

func NewBracketQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.BracketQty, true })
}

func NewMatchedBidQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.MatchedBidQty, true })
}

func NewMatchedAskQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.MatchedAskQty, true })
}

func NewFillBidQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.FillBidQty, true })
}

func NewFillAskQty() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.FillAskQty, true })
}

func NewFillBidFraction() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.FillBidFrac, true })
}

func NewFillAskFraction() core.Primitive {
	return newMatchPick(func(reading *MatchReading) (float64, bool) { return reading.FillAskFrac, true })
}
