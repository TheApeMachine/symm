package signal

import (
	"container/ring"
	"fmt"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

const minimumObservationSeconds = 1e-6

/*
UniformConfidence is the 1/N budget for a uniform guess across categoryCount classes.
*/
func UniformConfidence(categoryCount int) float64 {
	if categoryCount <= 0 {
		return 0
	}

	return 1.0 / float64(categoryCount)
}

/*
FinishMeasure returns a publishable candidate or a uniform best-effort guess from ring samples.
*/
func FinishMeasure(
	source logic.SourceType,
	symbol string,
	category logic.CategoryType,
	categoryCount int,
	measurements *ring.Ring,
	at time.Time,
	candidate logic.Measurement,
	candidateErr error,
) (logic.Measurement, error) {
	if candidateErr != nil {
		return logic.Measurement{}, candidateErr
	}

	if candidate.Publishable() {
		return candidate, nil
	}

	return BestEffort(source, symbol, category, categoryCount, measurements, at)
}

/*
BestEffort builds a publishable uniform-guess measurement from the latest ring samples.
*/
func BestEffort(
	source logic.SourceType,
	symbol string,
	category logic.CategoryType,
	categoryCount int,
	measurements *ring.Ring,
	at time.Time,
) (logic.Measurement, error) {
	if symbol == "" || categoryCount <= 0 {
		return logic.Measurement{}, fmt.Errorf("signal: best effort requires symbol and category count")
	}

	if at.IsZero() {
		return logic.Measurement{}, fmt.Errorf("signal: best effort requires observed time")
	}

	row, elapsed, volume, spread, quoteErr := RingQuote(symbol, measurements, at)

	if quoteErr != nil {
		return logic.Measurement{}, quoteErr
	}

	if row == nil {
		return logic.Measurement{}, nil
	}

	confidence := UniformConfidence(categoryCount)

	return logic.Measurement{
		Source:     source,
		Symbol:     symbol,
		Price:      row.Price,
		Strength:   confidence,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    elapsed,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   confidence,
		ObservedAt: at,
		Market:     *row,
	}, nil
}

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

	change := spread / price

	if change <= 0 {
		return nil, 0, 0, 0, nil
	}

	if volume <= 0 {
		volume = price
	}

	row, err := krakenmarket.NewSymbolRow(symbol, price, change, volume, 1, at)

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

		volume := event.Price * event.Qty

		if volume <= 0 {
			volume = event.Price
		}

		spread := event.Qty

		if spread <= 0 {
			spread = minimumObservationSeconds
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
