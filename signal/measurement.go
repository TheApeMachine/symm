package signal

import (
	"container/ring"
	"math"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

const (
	minimumObservationSeconds = 1e-6
	minimumPriceSpread        = 1e-8
)

/*
RingQuote returns market fields from the ring, falling back to a single touch quote.
*/
func RingQuote(
	symbol string,
	measurements *ring.Ring,
	at time.Time,
) (*krakenmarket.Symbol, float64, float64, float64, error) {
	row, elapsed, volume, spread, err := RingMarketRow(symbol, measurements, at)

	if row != nil {
		return row, elapsed, volume, spread, err
	}

	return ringTouchQuote(symbol, measurements, at)
}

func ringTouchQuote(
	symbol string,
	measurements *ring.Ring,
	at time.Time,
) (*krakenmarket.Symbol, float64, float64, float64, error) {
	if measurements == nil {
		return nil, 0, 0, 0, nil
	}

	var (
		price     float64
		spread    float64
		volume    float64
		anchor    time.Time
		quoteSeen bool
	)

	measurements.Do(func(item any) {
		if item == nil {
			return
		}

		quote, eventAt, ok := touchQuote(item)

		if !ok || quote.price <= 0 || quote.spread <= 0 {
			return
		}

		price = quote.price
		spread = quote.spread
		volume = quote.volume

		if eventAt.IsZero() {
			return
		}

		if anchor.IsZero() || eventAt.Before(anchor) {
			anchor = eventAt
		}

		quoteSeen = true
	})

	if !quoteSeen {
		return nil, 0, 0, 0, nil
	}

	elapsed := at.Sub(anchor).Seconds()

	if elapsed <= 0 {
		elapsed = minimumObservationSeconds
	}

	relativeSpread := spread / price

	// Guards against unexpected zero or NaN from upstream quote data.
	if relativeSpread <= 0 {
		return nil, 0, 0, 0, nil
	}

	if volume <= 0 {
		volume = price
	}

	row, err := krakenmarket.NewSymbolRow(symbol, price, relativeSpread, volume, 1, at)

	if err != nil {
		return nil, 0, 0, 0, err
	}

	return row, elapsed, volume, spread, nil
}

type touchQuoteReading struct {
	price  float64
	spread float64
	volume float64
}

func touchQuote(item any) (touchQuoteReading, time.Time, bool) {
	switch event := item.(type) {
	case *krakenmarket.TradeUpdate:
		if event.Price <= 0 {
			return touchQuoteReading{}, time.Time{}, false
		}

		// Notional volume in quote currency.
		volume := event.Price * event.Qty

		if volume <= 0 {
			volume = event.Price
		}

		spread := event.Qty

		if spread <= 0 {
			spread = math.Max(minimumPriceSpread, event.Price*1e-6)
		}

		return touchQuoteReading{
			price:  event.Price,
			spread: spread,
			volume: volume,
		}, event.Timestamp, true
	case *krakenmarket.TickerUpdate:
		price, err := event.ResolvePrice()

		if err != nil || price <= 0 {
			return touchQuoteReading{}, time.Time{}, false
		}

		touchSpread := event.Ask - event.Bid

		if touchSpread <= 0 {
			return touchQuoteReading{}, time.Time{}, false
		}

		volume := event.Volume

		if volume <= 0 {
			volume = event.AskQty + event.BidQty
		}

		if volume <= 0 {
			volume = price
		}

		// Notional volume in quote currency.
		return touchQuoteReading{
			price:  price,
			spread: touchSpread,
			volume: volume * price,
		}, event.Timestamp, true
	case *krakenmarket.BookUpdate:
		if len(event.Bids) == 0 || len(event.Asks) == 0 {
			return touchQuoteReading{}, time.Time{}, false
		}

		bid := event.Bids[0].Price
		ask := event.Asks[0].Price
		mid := (bid + ask) / 2
		touchSpread := ask - bid

		if mid <= 0 || touchSpread <= 0 {
			return touchQuoteReading{}, time.Time{}, false
		}

		// Notional volume in quote currency.
		volume := mid * (event.Bids[0].Qty + event.Asks[0].Qty)

		if volume <= 0 {
			volume = mid
		}

		return touchQuoteReading{
			price:  mid,
			spread: touchSpread,
			volume: volume,
		}, event.Timestamp, true
	default:
		return touchQuoteReading{}, time.Time{}, false
	}
}
