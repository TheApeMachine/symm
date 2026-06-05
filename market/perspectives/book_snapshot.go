package perspectives

import (
	"github.com/theapemachine/symm/kraken/market"
)

/*
BookLevel is one compact L2 price level stored on a measurement tape row.
Short JSON keys keep capture files smaller when full depth is recorded.
*/
type BookLevel struct {
	Price float64 `json:"p"`
	Qty   float64 `json:"q"`
}

/*
HasBookDepth reports whether the measurement carries walkable L2 liquidity.
*/
func (measurement Measurement) HasBookDepth() bool {
	return len(measurement.BookBids) > 0 || len(measurement.BookAsks) > 0
}

/*
AttachBook copies top-of-book and depth levels from a live quote into measurement.
depth limits how many levels are stored per side.
*/
func AttachBook(
	measurement Measurement,
	bid, ask, last float64,
	book market.Book,
	depth int,
) Measurement {
	if bid > 0 {
		measurement.Bid = bid
	}

	if ask > 0 {
		measurement.Ask = ask
	}

	if last > 0 {
		measurement.Last = last
	}

	if bid > 0 && ask > 0 && ask >= bid {
		mid := (bid + ask) / 2

		if mid > 0 {
			measurement.SpreadBPS = (ask - bid) / mid * 10_000
		}
	}

	measurement.BookBids = copyBookLevels(book.Bids, depth)
	measurement.BookAsks = copyBookLevels(book.Asks, depth)

	return measurement
}

func copyBookLevels(levels []market.BookLevel, depth int) []BookLevel {
	if len(levels) == 0 || depth <= 0 {
		return nil
	}

	if len(levels) < depth {
		depth = len(levels)
	}

	out := make([]BookLevel, depth)

	for index := 0; index < depth; index++ {
		out[index] = BookLevel{
			Price: levels[index].Price,
			Qty:   levels[index].Qty,
		}
	}

	return out
}

/*
MarketBook rebuilds a kraken market.Book from the compact tape snapshot.
*/
func (measurement Measurement) MarketBook() market.Book {
	book := market.Book{
		Bids: levelsToMarket(measurement.BookBids),
		Asks: levelsToMarket(measurement.BookAsks),
	}

	return book
}

func levelsToMarket(levels []BookLevel) []market.BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]market.BookLevel, len(levels))

	for index, level := range levels {
		out[index] = market.BookLevel{
			Price: level.Price,
			Qty:   level.Qty,
		}
	}

	return out
}
