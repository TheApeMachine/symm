package broker

import (
	symmmarket "github.com/theapemachine/symm/market"
)

func quoteFromTouch(touch symmmarket.TouchSnapshot) QuoteSnapshot {
	return QuoteSnapshot{
		Symbol:     touch.Symbol,
		Bid:        touch.Bid,
		Ask:        touch.Ask,
		Last:       touch.Last,
		ObservedAt: touch.ObservedAt,
	}
}
