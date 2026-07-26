package kraken

import (
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
		bids = append(bids, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    decimal.ExactDiv(&level.Price, row.PriceIncrement).Int64(),
			Quantity: level.Qty,
		})
	}

	for _, level := range row.Asks {
		asks = append(asks, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    decimal.ExactDiv(&level.Price, row.PriceIncrement).Int64(),
			Quantity: level.Qty,
		})
	}

	return bids, asks, nil
}
