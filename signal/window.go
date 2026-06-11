package signal

import (
	"container/ring"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
)

/*
ObservationElapsed returns seconds from the oldest ring sample to observedAt.
*/
func ObservationElapsed(measurements *ring.Ring, observedAt time.Time) (float64, error) {
	anchor, err := ringAnchor(measurements)

	if err != nil {
		return 0, err
	}

	if observedAt.IsZero() {
		return 0, fmt.Errorf("signal: observed time is required")
	}

	elapsed := observedAt.Sub(anchor).Seconds()

	if elapsed <= 0 {
		return 0, fmt.Errorf("signal: elapsed must be positive")
	}

	return elapsed, nil
}

func ringAnchor(measurements *ring.Ring) (time.Time, error) {
	if measurements == nil {
		return time.Time{}, fmt.Errorf("signal: measurement ring is nil")
	}

	var anchor time.Time

	measurements.Do(func(item any) {
		if item == nil {
			return
		}

		eventAt, ok := eventTime(item)

		if !ok || eventAt.IsZero() {
			return
		}

		if anchor.IsZero() || eventAt.Before(anchor) {
			anchor = eventAt
		}
	})

	if anchor.IsZero() {
		return time.Time{}, fmt.Errorf("signal: ring has no timestamped samples")
	}

	return anchor, nil
}

func eventTime(item any) (time.Time, bool) {
	switch event := item.(type) {
	case *krakenmarket.TradeUpdate:
		return event.Timestamp, true
	case *krakenmarket.TickerUpdate:
		return event.Timestamp, true
	case *krakenmarket.BookUpdate:
		return event.Timestamp, true
	default:
		return time.Time{}, false
	}
}

/*
TouchSpread returns median absolute price move as a spread proxy from prices.
*/
func TouchSpread(prices []float64) (float64, error) {
	if len(prices) < 2 {
		return 0, fmt.Errorf("signal: spread requires at least two prices")
	}

	moves := make([]float64, 0, len(prices)-1)

	for index := 1; index < len(prices); index++ {
		moves = append(moves, prices[index]-prices[index-1])
	}

	spread := numeric.MedianAbsolute(moves)

	if spread <= 0 {
		return 0, fmt.Errorf("signal: spread is required")
	}

	return spread, nil
}

/*
RingMarketRow builds a validated market row and observation fields from ring trades.
*/
func RingMarketRow(
	symbol string,
	measurements *ring.Ring,
	at time.Time,
) (*krakenmarket.Symbol, float64, float64, float64, error) {
	var (
		prices   []float64
		quoteVol float64
	)

	measurements.Do(func(item any) {
		if item == nil {
			return
		}

		switch event := item.(type) {
		case *krakenmarket.TradeUpdate:
			prices = append(prices, event.Price)
			quoteVol += event.Price * event.Qty
		case *krakenmarket.TickerUpdate:
			price, priceErr := event.ResolvePrice()

			if priceErr != nil {
				return
			}

			prices = append(prices, price)
			volume := event.Volume

			if volume <= 0 {
				volume = event.AskQty + event.BidQty
			}

			quoteVol += volume * price
		case *krakenmarket.BookUpdate:
			if len(event.Bids) == 0 || len(event.Asks) == 0 {
				return
			}

			bid := event.Bids[0].Price
			ask := event.Asks[0].Price
			mid := (bid + ask) / 2

			if mid <= 0 {
				return
			}

			prices = append(prices, mid)
			quoteVol += mid * (event.Bids[0].Qty + event.Asks[0].Qty)
		}
	})

	if len(prices) < 2 {
		return nil, 0, 0, 0, errnie.Error(fmt.Errorf("signal: insufficient window"))
	}

	elapsed, err := ObservationElapsed(measurements, at)

	if err != nil {
		return nil, 0, 0, 0, errnie.Error(err)
	}

	spread, err := TouchSpread(prices)

	if err != nil {
		return nil, 0, 0, 0, errnie.Error(err)
	}

	row, err := krakenmarket.SymbolRowFromPrices(symbol, prices, quoteVol, 1, at)

	if err != nil {
		return nil, 0, 0, 0, errnie.Error(err)
	}

	return row, elapsed, quoteVol, spread, nil
}
