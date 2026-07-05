package manifold

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

func (signal *Signal) observeBooks(rows kraken.BookDataSlice) error {
	for _, row := range rows {
		update := BookUpdate{
			Symbol:    row.Symbol,
			Type:      "snapshot",
			Timestamp: row.Timestamp,
			Bids:      bookLevels(row.Bids),
			Asks:      bookLevels(row.Asks),
		}

		if err := signal.observeBookUpdate(update); err != nil {
			return err
		}
	}

	return nil
}

func (signal *Signal) observeTrades(rows kraken.TradeDataSlice) error {
	for _, row := range rows {
		update := TradeUpdate{
			Symbol:    row.Symbol,
			Side:      row.Side,
			Price:     row.Price,
			Qty:       row.Qty,
			Timestamp: row.Timestamp,
		}

		if err := signal.observeTradeUpdate(update); err != nil {
			return err
		}
	}

	return nil
}

func (signal *Signal) observeTickers(rows kraken.TickerDataSlice) error {
	for _, row := range rows {
		update := TickerUpdate{
			Symbol:    row.Symbol,
			Last:      row.Last,
			Bid:       row.Bid,
			Ask:       row.Ask,
			BidQty:    row.BidQty,
			AskQty:    row.AskQty,
			Timestamp: row.Timestamp,
		}

		if err := signal.observeTickerUpdate(update); err != nil {
			return err
		}
	}

	return nil
}

func bookLevels(rows []kraken.BookLevel) []BookLevel {
	levels := make([]BookLevel, 0, len(rows))

	for _, row := range rows {
		levels = append(levels, BookLevel{
			Price: row.Price,
			Qty:   row.Qty,
		})
	}

	return levels
}

func (signal *Signal) observeBookUpdate(update BookUpdate) error {
	if update.Symbol == "" {
		return errnie.Err(errnie.Validation, "manifold: book symbol required", nil)
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	if err := signal.field.FeedBook(update, eventAt); err != nil {
		return errnie.Err(
			errnie.Validation,
			"manifold: book feed failed for "+update.Symbol,
			err,
		)
	}

	return nil
}

func (signal *Signal) observeTradeUpdate(update TradeUpdate) error {
	if update.Symbol == "" || update.Price <= 0 {
		return errnie.Err(errnie.Validation, "manifold: trade symbol and price required", nil)
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	row := update
	if err := signal.field.FeedTrade(&row, eventAt); err != nil {
		return errnie.Err(
			errnie.Validation,
			"manifold: trade feed failed for "+update.Symbol,
			err,
		)
	}

	return nil
}

func (signal *Signal) observeTickerUpdate(update TickerUpdate) error {
	if update.Symbol == "" {
		return errnie.Err(errnie.Validation, "manifold: ticker symbol required", nil)
	}

	price := update.Last

	if price <= 0 && update.Bid > 0 && update.Ask > update.Bid {
		price = (update.Bid + update.Ask) / 2
	}

	if price <= 0 {
		return errnie.Err(errnie.Validation, "manifold: ticker price required", nil)
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	row := update
	row.Last = price
	if err := signal.field.FeedTicker(row, eventAt); err != nil {
		return errnie.Err(
			errnie.Validation,
			"manifold: ticker feed failed for "+update.Symbol,
			err,
		)
	}

	return nil
}
