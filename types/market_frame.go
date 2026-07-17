package types

import (
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/symm/kraken"
)

/*
MarketFrame is one immutable cut of centrally decoded public market data.
Signals may read it concurrently; only the central feed constructs or mutates it.
*/
type MarketFrame struct {
	Tickers      []kraken.TickerData
	Trades       []kraken.TradeData
	Books        []kraken.BookData
	CrossSection *CrossSection
}

func (frame *MarketFrame) IsEmpty() bool {
	return len(frame.Tickers) == 0 &&
		len(frame.Trades) == 0 &&
		len(frame.Books) == 0
}

/*
BookLevels projects one decoded book row into tick-normalized bid and ask
levels. It is pure row conversion, so book-driven signals share one extraction
instead of each owning a legacy book cache.
*/
func BookLevels(
	row kraken.BookData,
) ([]flow.BookLevel, []flow.BookLevel, error) {
	bids := make([]flow.BookLevel, 0, len(row.Bids))
	asks := make([]flow.BookLevel, 0, len(row.Asks))

	for _, level := range row.Bids {
		tick, err := kraken.PriceTick(level.Price, *row.PriceIncrement)

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
		tick, err := kraken.PriceTick(level.Price, *row.PriceIncrement)

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
