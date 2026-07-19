package types

import (
	"fmt"
	"time"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/symm/kraken"
)

/*
MarketFrame is one immutable cut of centrally decoded public market data.
Signals may read it concurrently; only the central feed constructs or mutates it.
Advanced names which feeds progressed this cut — distinct from Tickers/Trades/
Books length, which may carry retained context for a non-advanced stream.
*/
type MarketFrame struct {
	At           time.Time
	Tickers      []kraken.TickerData
	Trades       []kraken.TradeData
	Books        []kraken.BookData
	CrossSection *CrossSection
	Advanced     StreamInterest
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
	if row.PriceIncrement == nil {
		return nil, nil, fmt.Errorf(
			"book %s: price increment is required",
			row.Symbol,
		)
	}

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
