package types

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
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
	ctx          context.Context
	cancel       context.CancelFunc
	subscribers  *sync.Map
	ui           chan []byte
	equityMu     sync.RWMutex
	equity       *kraken.TradeBalanceResult
	Status       Status        `json:"status"`
	Tick         int64         `json:"tick"`
	At           time.Time     `json:"at"`
	LastTickerAt time.Time     `json:"lastTickerAt"`
	LastTradeAt  time.Time     `json:"lastTradeAt"`
	CrossSection *CrossSection `json:"crossSection"`
	Measurements *sync.Map     `json:"-"`
	Symbols      *sync.Map     `json:"-"`
	Measured     *sync.Map     `json:"-"`
	Tickers      *sync.Map     `json:"-"`
	Trades       *sync.Map     `json:"-"`
	Lifecycle    *sync.Map     `json:"lifecycle"`
	Manifold     fluid.Reading `json:"-"`
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(
	ctx context.Context, ui chan []byte,
) *Thesis {
	ctx, cancel := context.WithCancel(ctx)

	return &Thesis{
		ctx:          ctx,
		cancel:       cancel,
		subscribers:  &sync.Map{},
		ui:           ui,
		Status:       READY,
		At:           time.Now().UTC(),
		LastTickerAt: time.Now().UTC(),
		LastTradeAt:  time.Now().UTC(),
		CrossSection: NewCrossSection(),
		Lifecycle:    &sync.Map{},
		Measurements: &sync.Map{},
		Symbols:      &sync.Map{},
		Measured:     &sync.Map{},
		Tickers:      &sync.Map{},
		Trades:       &sync.Map{},
		Manifold:     fluid.Reading{},
	}
}

/*
Reset clears completed symbol evaluations. With no symbols it clears the full
market state.
*/
func (thesis *Thesis) Reset(symbols ...string) *Thesis {
	if len(symbols) > 0 {
		for _, symbolName := range symbols {
			value, found := thesis.Symbols.Load(symbolName)

			if found {
				symbol, ok := value.(*Symbol)

				if ok && symbol != nil {
					symbol.Reset()
				}
			}

			thesis.Tickers.Delete(symbolName)
			thesis.Trades.Delete(symbolName)
			thesis.Symbols.Delete(symbolName)
		}

		thesis.At = time.Now().UTC()
		return thesis
	}

	thesis.Symbols.Range(func(_, value any) bool {
		value.(*Symbol).Reset()
		return true
	})
	thesis.At = time.Now().UTC()
	thesis.CrossSection = NewCrossSection()
	thesis.Tickers.Clear()
	thesis.Trades.Clear()
	thesis.Manifold = fluid.Reading{}
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

			for index, existingTicker := range tickers {
				if ticker.Timestamp.Before(existingTicker.Timestamp) {
					// Insert the new ticker before the existing ticker.
					tickers = append(tickers[:index], append([]kraken.TickerData{ticker}, tickers[index:]...)...)
					thesis.Tickers.Store(ticker.Symbol, tickers)
					thesis.Fanout(SourceTrader, tickerReceivers...)
					return thesis
				}
			}
		}

		thesis.Tickers.Store(ticker.Symbol, append(found.([]kraken.TickerData), ticker))
	}

	thesis.LastTickerAt = ticker.Timestamp
	thesis.Fanout(SourceTrader, tickerReceivers...)

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

			for index, existingTrade := range trades {
				if trade.Timestamp.Before(existingTrade.Timestamp) {
					// Insert the new trade before the existing trade.
					trades = append(trades[:index], append([]kraken.TradeData{trade}, trades[index:]...)...)
					thesis.Trades.Store(trade.Symbol, trades)
					thesis.Fanout(SourceTrader, tradeReceivers...)
					return thesis
				}
			}
		}

		thesis.Trades.Store(trade.Symbol, append(found.([]kraken.TradeData), trade))
	}

	thesis.LastTradeAt = trade.Timestamp
	thesis.Fanout(SourceTrader, tradeReceivers...)

	return thesis
}

/*
AppendEquity retains the latest complete account valuation and wakes only the
global regulator. Account feedback must not start another market-analysis pass.
*/
func (thesis *Thesis) AppendEquity(equity kraken.TradeBalanceResult) error {
	if equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"thesis: positive account equity required",
			nil,
		))
	}

	thesis.equityMu.Lock()
	thesis.equity = &equity
	thesis.equityMu.Unlock()
	thesis.Fanout(SourceEquity, SourceRegulator)

	return nil
}

/*
Equity returns the latest complete account valuation received from the broker.
*/
func (thesis *Thesis) Equity() (kraken.TradeBalanceResult, bool) {
	thesis.equityMu.RLock()
	defer thesis.equityMu.RUnlock()

	if thesis.equity == nil {
		return kraken.TradeBalanceResult{}, false
	}

	return *thesis.equity, true
}

func (thesis *Thesis) AppendMeasurements(
	sender SourceType,
	measurements []*Measurement,
	ready bool,
) error {
	shouldFanout := false

	if len(measurements) > 0 {
		found, ok := thesis.Measurements.LoadOrStore(sender, measurements)

		if ok {
			stored := found.([]*Measurement)

			for i := range stored {
				symbolFound, ok := thesis.Symbols.Load(stored[i].Symbol)

				if ok && symbolFound != nil {
					symbol, ok := symbolFound.(*Symbol)

					if ok && symbol != nil && symbol.Status == READY {
						symbol.AddMeasurement(stored[i])
						stored = slices.Delete(stored, i, i+1)
						shouldFanout = true
					}
				}
			}

			stored = append(stored, measurements...)
			thesis.Measurements.Store(sender, stored)
		}
	}

	if ready && len(measurements) > 0 && shouldFanout {
		thesis.Fanout(
			sender,
			SourceAnalyzer,
		)
	}

	return nil
}

/*
MarketTickers returns all the tickers in the thesis, except those that the
source has already seen. This is used to fan out new tickers to subscribers.
*/
func (thesis *Thesis) MarketTickers(source SourceType) []kraken.TickerData {
	out := make([]kraken.TickerData, 0)
	latestAt := time.Time{}

	stored, ok := thesis.Measured.Load(source + "tickers")

	if ok {
		latestAt, _ = stored.(time.Time)
	}

	thesis.Tickers.Range(func(key, value any) bool {
		if tickerSlice, ok := value.([]kraken.TickerData); ok {
			for _, ticker := range tickerSlice {
				if !latestAt.IsZero() && ticker.Timestamp.Before(latestAt) {
					continue
				}

				out = append(out, ticker)

				if ticker.Timestamp.After(latestAt) {
					thesis.Measured.Store(source+"tickers", ticker.Timestamp)
					latestAt = ticker.Timestamp
				}
			}
		}

		return true
	})

	return out
}

func (thesis *Thesis) MarketTrades(source SourceType) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)
	latestAt := time.Time{}

	stored, ok := thesis.Measured.Load(source + "trades")

	if ok {
		latestAt, _ = stored.(time.Time)
	}

	thesis.Trades.Range(func(key, value any) bool {
		if tradeSlice, ok := value.([]kraken.TradeData); ok {
			for _, trade := range tradeSlice {
				if !latestAt.IsZero() && trade.Timestamp.Before(latestAt) {
					continue
				}

				out = append(out, trade)

				if trade.Timestamp.After(latestAt) {
					thesis.Measured.Store(source+"trades", trade.Timestamp)
					latestAt = trade.Timestamp
				}
			}
		}

		return true
	})

	return out
}

func (thesis *Thesis) Subscribe(source SourceType, semaphore chan struct{}) {
	thesis.subscribers.Store(source, semaphore)
}

/*
Fanout sends a wake-up signal to all subscribers of the thesis, except for the sender.
If specific receivers are provided, it will only fan out to those receivers.
*/
func (thesis *Thesis) Fanout(sender SourceType, receivers ...SourceType) {
	thesis.subscribers.Range(func(key, value any) bool {
		source := key.(SourceType)

		// Prevent the sender from receiving its own fanout, and if receivers
		// are specified, only fan out to those receivers.
		if source == sender || (len(receivers) > 0 && !slices.Contains(receivers, source)) {
			return true
		}

		semaphore := value.(chan struct{})

		select {
		case <-thesis.ctx.Done():
			return false
		case semaphore <- struct{}{}:
		default:
			errnie.Warn(fmt.Sprintf(
				"thesis: fanout to source [%s] skipped, semaphore full", source,
			))
		}

		return true
	})
}

/*
Stamp marks the symbol as having been measured by the source. This is used to
track which sources have contributed to the symbol's measurements and to ensure
that all sources have been accounted for before making decisions based on the
symbol's measurements.
*/
func (thesis *Thesis) Stamp(symbol string, source SourceType) {
	found, ok := thesis.Symbols.Load(symbol)

	if !ok {
		return
	}

	if s, ok := found.(*Symbol); ok && s != nil {
		s.Stamp(source)
	}
}

/*
Stamped returns true if the symbol has been stamped by the thesis for all
specified sources. If no sources are specified, it checks if the symbol has been
stamped for all sources.
*/
func (thesis *Thesis) Stamped(symbol string, sources ...SourceType) bool {
	symbolStamped := false

	found, ok := thesis.Symbols.Load(symbol)

	if !ok {
		return false
	}

	if s, ok := found.(*Symbol); ok && s != nil {
		if len(sources) == 0 {
			symbolStamped = s.Readiness.Complete()
		} else {
			symbolStamped = true

			for _, source := range sources {
				if !s.Readiness.Stamped(source) {
					symbolStamped = false
					break
				}
			}
		}
	}

	return symbolStamped
}

func (thesis *Thesis) SymbolsReady() bool {
	ready := true

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*Symbol)

		if !ok || symbol == nil {
			return true
		}

		if symbol.Status != READY {
			ready = false
			return false
		}

		return true
	})

	return ready
}

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
