package broker

import (
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
)

/*
MicroPrice is Stoikov's high-frequency fair-price estimator: mid adjusted by
top-of-book queue imbalance so wide micro-cap spreads do not bounce CVD /
morphology off a naive mid.
*/
func (price *Price) MicroPrice(symbol string) (*decimal.Decimal, bool) {
	if price == nil {
		return nil, false
	}

	name := price.normalizer.Name(symbol)
	var out *decimal.Decimal

	price.Books.Book(name, func(book *market.Book) {
		if !book.Tradable() {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()
		if bid == nil || ask == nil ||
			bid.Price == nil || ask.Price == nil ||
			bid.Quantity == nil || ask.Quantity == nil {
			return
		}

		bidPx := *bid.Price
		askPx := *ask.Price
		bidQty := *bid.Quantity
		askQty := *ask.Quantity
		if bidPx.Sign() <= 0 || askPx.Sign() <= 0 ||
			bidQty.Sign() <= 0 || askQty.Sign() <= 0 ||
			askPx.Cmp(bidPx) <= 0 {
			return
		}

		// micro = (askQty·bid + bidQty·ask) / (bidQty + askQty)
		den := bidQty.Add(askQty)
		micro := askQty.Mul(bidPx).Add(bidQty.Mul(askPx)).Div(den)
		out = &micro
	})

	if out == nil {
		return nil, false
	}

	return out, true
}
