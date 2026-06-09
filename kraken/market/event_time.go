package market

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/logic"
)

/*
EventTimeFromTrade returns the exchange timestamp on a trade update.
*/
func EventTimeFromTrade(trade *TradeUpdate) (time.Time, error) {
	if trade == nil {
		return time.Time{}, fmt.Errorf("kraken/market: trade is nil")
	}

	if trade.Timestamp.IsZero() {
		return time.Time{}, fmt.Errorf("kraken/market: trade %q timestamp is zero", trade.Symbol)
	}

	return trade.Timestamp, nil
}

/*
EventTimeFromBook parses the RFC3339 timestamp on a book frame.
*/
func EventTimeFromBook(book *Book) (time.Time, error) {
	if book == nil {
		return time.Time{}, fmt.Errorf("kraken/market: book is nil")
	}

	return parseFeedTimestamp(book.Symbol, "book", book.Timestamp)
}

/*
EventTimeFromTicker parses the RFC3339 timestamp on a ticker row.
*/
func EventTimeFromTicker(ticker *TickerUpdate) (time.Time, error) {
	if ticker == nil {
		return time.Time{}, fmt.Errorf("kraken/market: ticker is nil")
	}

	return parseFeedTimestamp(ticker.Symbol, "ticker", ticker.Timestamp)
}

/*
EventTimeFromLevel3 returns the latest exchange timestamp across all orders in a level3 frame.
*/
func EventTimeFromLevel3(update *Level3Update) (time.Time, error) {
	if update == nil {
		return time.Time{}, fmt.Errorf("kraken/market: level3 update is nil")
	}

	var latest time.Time

	for _, bid := range update.Bids {
		if bid.Timestamp.IsZero() {
			return time.Time{}, fmt.Errorf(
				"kraken/market: level3 %q bid %q timestamp is zero",
				update.Symbol,
				bid.OrderID,
			)
		}

		if latest.IsZero() || bid.Timestamp.After(latest) {
			latest = bid.Timestamp
		}
	}

	for _, ask := range update.Asks {
		if ask.Timestamp.IsZero() {
			return time.Time{}, fmt.Errorf(
				"kraken/market: level3 %q ask %q timestamp is zero",
				update.Symbol,
				ask.OrderID,
			)
		}

		if latest.IsZero() || ask.Timestamp.After(latest) {
			latest = ask.Timestamp
		}
	}

	if latest.IsZero() {
		return time.Time{}, fmt.Errorf(
			"kraken/market: level3 %q has no order timestamps",
			update.Symbol,
		)
	}

	return latest, nil
}

func parseFeedTimestamp(symbol, channel, raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf(
			"kraken/market: %s %q timestamp is empty",
			channel,
			symbol,
		)
	}

	parsed, err := time.Parse(time.RFC3339Nano, raw)

	if err == nil {
		return parsed, nil
	}

	parsed, err = time.Parse(time.RFC3339, raw)

	if err != nil {
		return time.Time{}, fmt.Errorf(
			"kraken/market: %s %q timestamp %q: %w",
			channel,
			symbol,
			raw,
			err,
		)
	}

	return parsed, nil
}

/*
EventTimeFromBus extracts the exchange timestamp from a raw bus row.
*/
func EventTimeFromBus(messageType string, value any) (time.Time, error) {
	switch messageType {
	case "trades":
		trade, ok := value.(*TradeUpdate)

		if !ok {
			return time.Time{}, fmt.Errorf("kraken/market: bus trades value is not *TradeUpdate")
		}

		return EventTimeFromTrade(trade)
	case "ticker":
		ticker, ok := value.(*TickerUpdate)

		if !ok {
			return time.Time{}, fmt.Errorf("kraken/market: bus ticker value is not *TickerUpdate")
		}

		return EventTimeFromTicker(ticker)
	case "book":
		book, ok := value.(*Book)

		if !ok {
			return time.Time{}, fmt.Errorf("kraken/market: bus book value is not *Book")
		}

		return EventTimeFromBook(book)
	case "level3":
		switch update := value.(type) {
		case *Level3Update:
			return EventTimeFromLevel3(update)
		case Level3Update:
			return EventTimeFromLevel3(&update)
		default:
			return time.Time{}, fmt.Errorf("kraken/market: bus level3 value is not Level3Update")
		}
	case "measurements":
		measurement, ok := value.(logic.Measurement)

		if !ok {
			return time.Time{}, fmt.Errorf("kraken/market: bus measurements value is not logic.Measurement")
		}

		if measurement.ObservedAt.IsZero() {
			return time.Time{}, fmt.Errorf(
				"kraken/market: measurement %q/%s observed_at is zero",
				measurement.Symbol,
				measurement.Source,
			)
		}

		return measurement.ObservedAt, nil
	default:
		return time.Time{}, fmt.Errorf("kraken/market: bus message type %q has no event time", messageType)
	}
}
