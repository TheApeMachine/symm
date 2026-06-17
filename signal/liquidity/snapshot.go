package liquidity

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
)

type symbolBaseline struct {
	tracker *adaptive.TimeElasticMemory
}

/*
Metrics tracks per-symbol volume baselines for depth scoring.
*/
type Metrics struct {
	baselineWindow time.Duration
	epsilon        float64
	baselines      sync.Map
}

/*
NewMetrics returns a metrics store with time-elastic volume baselines.
*/
func NewMetrics() *Metrics {
	return &Metrics{
		baselineWindow: time.Minute,
		epsilon:        0.000001,
	}
}

func (metrics *Metrics) SeedBaseline(symbol string, at time.Time, volume float64) error {
	state := metrics.ensure(symbol)

	if state == nil {
		return fmt.Errorf("liquidity: invalid baseline state")
	}

	_, err := state.tracker.Update(at, volume)

	return err
}

func (metrics *Metrics) ensure(symbol string) *symbolBaseline {
	raw, _ := metrics.baselines.LoadOrStore(symbol, &symbolBaseline{
		tracker: adaptive.NewTimeElasticMemory(metrics.baselineWindow, metrics.epsilon),
	})

	state, ok := raw.(*symbolBaseline)

	if !ok {
		return nil
	}

	return state
}

func (metrics *Metrics) observeVolume(
	symbol string,
	at time.Time,
	volume float64,
) (relative float64, ready bool, err error) {
	if volume <= 0 {
		return 0, false, nil
	}

	state := metrics.ensure(symbol)

	if state == nil {
		return 0, false, fmt.Errorf("liquidity: invalid baseline state")
	}

	ready = state.tracker.Initialized()
	relative, err = state.tracker.Update(at, volume)

	return relative, ready, err
}

type ScopeSnapshot struct {
	Price    float64
	Volume   float64
	Spread   float64
	Elapsed  float64
	Observed time.Time
}

func scopeSnapshot(
	crossSection *marketsection.CrossSection,
	scope string,
	trade *feed.Trade,
	ticker *feed.Ticker,
	book *feed.Book,
) ScopeSnapshot {
	tickerSnap := ticker.Snapshot(scope)

	if tickerSnap.Last > 0 && tickerSnap.Bid > 0 && tickerSnap.Ask > tickerSnap.Bid {
		volume := tickerSnap.Volume

		if volume <= 0 {
			volume = tickerSnap.Last
		}

		return ScopeSnapshot{
			Price:    tickerSnap.Last,
			Volume:   volume,
			Spread:   tickerSnap.Ask - tickerSnap.Bid,
			Observed: tickerSnap.Observed,
		}
	}

	tradeSnap := tradeScopeSnapshot(crossSection, scope, trade)

	if tradeSnap.Price > 0 && tradeSnap.Volume > 0 {
		return tradeSnap
	}

	return bookScopeSnapshot(crossSection, scope, book)
}

func tradeScopeSnapshot(
	crossSection *marketsection.CrossSection,
	scope string,
	trade *feed.Trade,
) ScopeSnapshot {
	window, ok := trade.Window(scope)

	if !ok {
		return ScopeSnapshot{}
	}

	quoteVol := 0.0

	trade.Scan(scope, func(element []byte) {
		price, priceOK := feed.PeekElementOK[float64](element, "price")
		qty, qtyOK := feed.PeekElementOK[float64](element, "qty")

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return
		}

		quoteVol += price * qty
	})

	if quoteVol <= 0 {
		return ScopeSnapshot{}
	}

	spread, spreadOK := feed.TouchSpread(window.Prices)

	if !spreadOK {
		return ScopeSnapshot{}
	}

	tradeSnap := trade.Snapshot(scope)

	row, rowErr := krakenmarket.SymbolRowFromPrices(
		scope,
		window.Prices,
		quoteVol,
		1,
		tradeSnap.Observed,
	)

	if rowErr == nil {
		_ = crossSection.Observe(row)
	}

	return ScopeSnapshot{
		Price:    tradeSnap.Price,
		Volume:   quoteVol,
		Spread:   spread,
		Elapsed:  window.Elapsed,
		Observed: tradeSnap.Observed,
	}
}

func bookScopeSnapshot(
	crossSection *marketsection.CrossSection,
	scope string,
	book *feed.Book,
) ScopeSnapshot {
	window, ok := book.Window(scope)

	if !ok {
		return ScopeSnapshot{}
	}

	price := window.Prices[len(window.Prices)-1]
	spread := window.Spreads[len(window.Spreads)-1]
	bidQty, _ := feed.PeekElementOK[float64](window.LatestElement, "bids.0.qty")
	askQty, _ := feed.PeekElementOK[float64](window.LatestElement, "asks.0.qty")
	depth := bidQty + askQty
	quoteVol := depth * price
	observed, _ := feed.ElementTime(window.LatestElement, "timestamp")

	if quoteVol <= 0 {
		return ScopeSnapshot{}
	}

	row, rowErr := krakenmarket.SymbolRowFromPrices(
		scope,
		window.Prices,
		quoteVol,
		1,
		observed,
	)

	if rowErr == nil {
		_ = crossSection.Observe(row)
	}

	return ScopeSnapshot{
		Price:    price,
		Volume:   quoteVol,
		Spread:   spread,
		Observed: observed,
	}
}
