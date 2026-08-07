package types

import (
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/kraken"
)

const (
	ThesisKey = "thesis"

	LifecycleObserving           = "observing"
	LifecycleShaped              = "shaped"
	LifecycleEntrySelected       = "entry_selected"
	LifecycleEntrySubmitted      = "entry_submitted"
	LifecyclePartiallyEntered    = "partially_entered"
	LifecycleEntered             = "entered"
	LifecycleManaging            = "managing"
	LifecycleExitSelected        = "exit_selected"
	LifecycleExitSubmitted       = "exit_submitted"
	LifecyclePartiallyExited     = "partially_exited"
	LifecycleClosed              = "closed"
	LifecyclePostExitObservation = "post_exit_observation"
	LifecyclePostMortemReady     = "postmortem_ready"
	LifecycleEvaluated           = "evaluated"
	LifecycleExpired             = "expired"
	LifecycleRejected            = "rejected"
	LifecycleInvalid             = "invalid"
)

/*
Thesis owns canonical evidence across every evaluated epoch that contributes to
one decision. It closes only after the planner emits the completed decision set;
broker execution and settlement continue in their own lifecycle.
*/
type Thesis struct {
	Readiness
	Status       Status        `json:"status"`
	Tick         int64         `json:"tick"`
	At           time.Time     `json:"at"`
	LastTickerAt time.Time     `json:"lastTickerAt"`
	LastTradeAt  time.Time     `json:"lastTradeAt"`
	CrossSection *CrossSection `json:"crossSection"`
	Measurements *sync.Map     `json:"-"`
	Tickers      *sync.Map     `json:"-"`
	Trades       *sync.Map     `json:"-"`
	Graphs       *sync.Map     `json:"-"`
	Decisions    *sync.Map     `json:"decisions"`
	Lifecycle    *sync.Map     `json:"lifecycle"`
	Categories   *sync.Map     `json:"categories"`
	Manifold     fluid.Reading `json:"-"`
	Cognition    *sync.Map     `json:"-"`
	Resonance    *sync.Map     `json:"-"`
	Causal       *sync.Map     `json:"-"`
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(ui chan []byte) *Thesis {
	return &Thesis{
		Status:       READY,
		At:           time.Now().UTC(),
		LastTickerAt: time.Now().UTC(),
		LastTradeAt:  time.Now().UTC(),
		Readiness:    NewReadiness(ui),
		CrossSection: NewCrossSection(),
		Decisions:    &sync.Map{},
		Graphs:       &sync.Map{},
		Lifecycle:    &sync.Map{},
		Categories:   &sync.Map{},
		Measurements: &sync.Map{},
		Tickers:      &sync.Map{},
		Trades:       &sync.Map{},
		Manifold:     fluid.Reading{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
	}
}

/*
Reset the thesis to a clean state for the next evaluation.
This is only called once the readiness has reached the
Complete state and the planner has emitted actionable decisions.
*/
func (thesis *Thesis) Reset() *Thesis {
	if !thesis.Readiness.Complete() {
		return thesis
	}

	thesis.Readiness.Reset()
	thesis.At = time.Now().UTC()
	thesis.LastTickerAt = time.Now().UTC()
	thesis.LastTradeAt = time.Now().UTC()
	thesis.CrossSection = NewCrossSection()
	thesis.Tickers.Clear()
	thesis.Trades.Clear()
	thesis.Measurements.Clear()
	thesis.Categories.Clear()
	thesis.Cognition.Clear()
	thesis.Manifold = fluid.Reading{}
	thesis.Resonance.Clear()
	thesis.Causal.Clear()
	thesis.Graphs.Clear()
	thesis.Decisions.Clear()

	return thesis
}

func (thesis *Thesis) AppendTicker(ticker kraken.TickerData) *Thesis {
	found, ok := thesis.Tickers.LoadOrStore(ticker.Symbol, []kraken.TickerData{ticker})

	if ok {
		// Check if the ticker timestamp is after the last ticker timestamp.
		// If not, we need to insert the ticker in the correct position in
		// the slice to maintain chronological order.
		if ticker.Timestamp.After(thesis.LastTickerAt) {
			tickers := found.([]kraken.TickerData)

			for i, existingTicker := range tickers {
				if ticker.Timestamp.Before(existingTicker.Timestamp) {
					// Insert the new ticker before the existing ticker.
					tickers = append(tickers[:i], append([]kraken.TickerData{ticker}, tickers[i:]...)...)
					thesis.Tickers.Store(ticker.Symbol, tickers)
					return thesis
				}
			}
		}

		thesis.Tickers.Store(ticker.Symbol, append(found.([]kraken.TickerData), ticker))
		thesis.LastTickerAt = ticker.Timestamp
	}

	return thesis
}

func (thesis *Thesis) AppendTrade(trade kraken.TradeData) *Thesis {
	found, ok := thesis.Trades.LoadOrStore(trade.Symbol, []kraken.TradeData{trade})

	if ok {
		// Check if the trade timestamp is after the last trade timestamp.
		// If not, we need to insert the trade in the correct position in
		// the slice to maintain chronological order.
		if trade.Timestamp.After(thesis.LastTradeAt) {
			trades := found.([]kraken.TradeData)

			for i, existingTrade := range trades {
				if trade.Timestamp.Before(existingTrade.Timestamp) {
					// Insert the new trade before the existing trade.
					trades = append(trades[:i], append([]kraken.TradeData{trade}, trades[i:]...)...)
					thesis.Trades.Store(trade.Symbol, trades)
					return thesis
				}
			}
		}

		thesis.Trades.Store(trade.Symbol, append(found.([]kraken.TradeData), trade))
		thesis.LastTradeAt = trade.Timestamp
	}

	return thesis
}

func (thesis *Thesis) AppendMeasurements(
	measurements []*Measurement, ready bool,
) *Thesis {
	if len(measurements) == 0 {
		return thesis
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		found, ok := thesis.Measurements.LoadOrStore(
			measurement.Source,
			[]*Measurement{measurement},
		)

		if ok {
			thesis.Measurements.Store(measurement.Source, append(found.([]*Measurement), measurement))
		}

		if ready {
			thesis.Readiness.Stamp(SourceType(measurement.Source))
		}
	}

	return thesis
}

func (thesis *Thesis) LatestTicker(symbol string) (kraken.TickerData, bool) {
	tickersRaw, found := thesis.Tickers.Load(symbol)

	if !found {
		return kraken.TickerData{}, false
	}

	tickers := tickersRaw.([]kraken.TickerData)

	if len(tickers) == 0 {
		return kraken.TickerData{}, false
	}

	return tickers[len(tickers)-1], true
}

func (thesis *Thesis) LatestTrade(symbol string) (kraken.TradeData, bool) {
	tradesRaw, found := thesis.Trades.Load(symbol)

	if !found {
		return kraken.TradeData{}, false
	}

	trades := tradesRaw.([]kraken.TradeData)

	if len(trades) == 0 {
		return kraken.TradeData{}, false
	}

	return trades[len(trades)-1], true
}

func (thesis *Thesis) MarketSymbols() []string {
	symbols := make([]string, 0)

	thesis.Tickers.Range(func(key, value any) bool {
		if symbol, ok := key.(string); ok {
			if !slices.Contains(symbols, symbol) {
				symbols = append(symbols, symbol)
			}
		}

		return true
	})

	return symbols
}

func (thesis *Thesis) MarketTickers() []kraken.TickerData {
	tickers := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(key, value any) bool {
		if tickerSlice, ok := value.([]kraken.TickerData); ok {
			tickers = append(tickers, tickerSlice...)
		}

		return true
	})

	return tickers
}

func (thesis *Thesis) MarketTrades() []kraken.TradeData {
	trades := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(key, value any) bool {
		if tradeSlice, ok := value.([]kraken.TradeData); ok {
			trades = append(trades, tradeSlice...)
		}

		return true
	})

	return trades
}

func (thesis *Thesis) Series(symbol string) []*Measurement {
	out := make([]*Measurement, 0)

	thesis.Measurements.Range(func(key, value any) bool {
		measurements, ok := value.([]*Measurement)

		if !ok {
			return true
		}

		for _, measurement := range measurements {
			if measurement.Symbol == symbol {
				out = append(out, measurement)
			}
		}

		return true
	})

	return out
}
