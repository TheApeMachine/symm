package kraken

import (
	"fmt"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
)

/*
BookLevels projects one decoded book row into tick-normalized bid and ask
levels.
*/
func BookLevels(row BookData) ([]flow.BookLevel, []flow.BookLevel, error) {
	if row.PriceIncrement == nil {
		return nil, nil, fmt.Errorf(
			"book %s: price increment is required",
			row.Symbol,
		)
	}

	bids := make([]flow.BookLevel, 0, len(row.Bids))
	asks := make([]flow.BookLevel, 0, len(row.Asks))

	for _, level := range row.Bids {
		tick, err := PriceTick(level.Price, *row.PriceIncrement)

		if err != nil {
			return nil, nil, err
		}

		bids = append(bids, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    tick,
			Quantity: level.Qty,
		})
	}

	for _, level := range row.Asks {
		tick, err := PriceTick(level.Price, *row.PriceIncrement)

		if err != nil {
			return nil, nil, err
		}

		asks = append(asks, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    tick,
			Quantity: level.Qty,
		})
	}

	return bids, asks, nil
}
