package types

import (
	"context"
	"fmt"
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
	Readiness
	ctx          context.Context
	cancel       context.CancelFunc
	subscribers  *sync.Map
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
	Graphs       *sync.Map     `json:"-"`
	Decisions    *sync.Map     `json:"decisions"`
	Lifecycle    *sync.Map     `json:"lifecycle"`
	Categories   *sync.Map     `json:"categories"`
	Manifold     fluid.Reading `json:"-"`
	Phase        *sync.Map     `json:"-"`
	Cognition    *sync.Map     `json:"-"`
	Resonance    *sync.Map     `json:"-"`
	Causal       *sync.Map     `json:"-"`
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
		Symbols:      &sync.Map{},
		Measured:     &sync.Map{},
		Tickers:      &sync.Map{},
		Trades:       &sync.Map{},
		Manifold:     fluid.Reading{},
		Phase:        &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
	}
}

/*
Reset starts the next evaluation epoch after the planner has completed this
one. Measurements remain as bounded, source-keyed prior evidence: each signal
replaces matching rows when it next produces an artifact. Raw market input and
derived decision artifacts are epoch-local and are cleared.
*/
func (thesis *Thesis) Reset() *Thesis {
	if !thesis.Readiness.Complete() {
		return thesis
	}

	thesis.Readiness.Reset()
	thesis.Symbols.Range(func(_, value any) bool {
		value.(*Symbol).Reset()
		return true
	})
	thesis.At = time.Now().UTC()
	thesis.LastTickerAt = time.Now().UTC()
	thesis.LastTradeAt = time.Now().UTC()
	thesis.CrossSection = NewCrossSection()
	thesis.Tickers.Clear()
	thesis.Trades.Clear()
	thesis.Categories.Clear()
	thesis.Cognition.Clear()
	thesis.Manifold = fluid.Reading{}
	thesis.Phase.Clear()
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
		thesis.Fanout(SourceTrader)
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
		thesis.Fanout(SourceTrader)
	}

	return thesis
}

func (thesis *Thesis) AppendMeasurements(
	sender SourceType,
	measurements []*Measurement,
	ready bool,
) error {
	if len(measurements) > 0 {
		found, ok := thesis.Measurements.LoadOrStore(sender, measurements)

		if ok {
			stored := found.([]*Measurement)

			for _, newMeasurement := range measurements {
				replaced := false

				for index, measurement := range stored {
					if measurement.ID == newMeasurement.ID {
						errnie.Error(errnie.Err(
							errnie.Conflict,
							fmt.Sprintf(
								"thesis: duplicate measurement found for [%s]",
								sender,
							),
							nil,
						))

						continue
					}

					if measurement.Source == newMeasurement.Source &&
						measurement.Symbol == newMeasurement.Symbol &&
						measurement.Peer == newMeasurement.Peer {
						stored[index] = newMeasurement
						replaced = true
					}
				}

				if !replaced {
					stored = append(stored, newMeasurement)
				}
			}

			thesis.Measurements.Store(sender, stored)
		}

		for _, measurement := range measurements {
			found, _ := thesis.Symbols.LoadOrStore(measurement.Symbol, &Symbol{})
			symbol, ok := found.(*Symbol)

			if !ok || symbol == nil {
				symbol = &Symbol{}
				thesis.Symbols.Store(measurement.Symbol, symbol)
			}

			replaced := false

			for index, stored := range symbol.Measurements {
				if stored != nil && stored.Source == measurement.Source && stored.Peer == measurement.Peer {
					symbol.Measurements[index] = measurement
					replaced = true
					break
				}
			}

			if !replaced {
				symbol.Measurements = append(symbol.Measurements, measurement)
			}
		}
	}

	if ready && len(measurements) > 0 {
		thesis.Readiness.Stamp(sender)
		thesis.Fanout(sender)
	}

	return nil
}

type tradeCursor struct {
	at      time.Time
	tradeID int64
}

type tickerCursor struct {
	at     time.Time
	symbol string
}

/*
MarketTickers returns all the tickers in the thesis, except those that the
source has already seen. This is used to fan out new tickers to subscribers.
*/
func (thesis *Thesis) MarketTickers(source SourceType) []kraken.TickerData {
	out := make([]kraken.TickerData, 0)
	latestAt := time.Time{}
	cursorAt := time.Time{}
	cursorSymbol := ""

	stored, ok := thesis.Measured.Load(source + "tickers")

	if ok {
		if tc, ok := stored.(tickerCursor); ok {
			cursorAt = tc.at
			cursorSymbol = tc.symbol
		} else if t, ok := stored.(time.Time); ok {
			cursorAt = t
		}
	}

	thesis.Tickers.Range(func(key, value any) bool {
		if tickerSlice, ok := value.([]kraken.TickerData); ok {
			for _, ticker := range tickerSlice {
				if ticker.Timestamp.Before(cursorAt) {
					continue
				}

				if ticker.Timestamp.Equal(cursorAt) && cursorSymbol != "" && ticker.Symbol == cursorSymbol {
					continue
				}

				out = append(out, ticker)

				if ticker.Timestamp.After(latestAt) || (ticker.Timestamp.Equal(latestAt) && ticker.Symbol != "") {
					thesis.Measured.Store(source+"tickers", tickerCursor{at: ticker.Timestamp, symbol: ticker.Symbol})
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
	var latestID int64
	cursorAt := time.Time{}
	var cursorID int64

	stored, ok := thesis.Measured.Load(source + "trades")

	if ok {
		if tc, ok := stored.(tradeCursor); ok {
			cursorAt = tc.at
			cursorID = tc.tradeID
		} else if t, ok := stored.(time.Time); ok {
			cursorAt = t
		}
	}

	thesis.Trades.Range(func(key, value any) bool {
		if tradeSlice, ok := value.([]kraken.TradeData); ok {
			for _, trade := range tradeSlice {
				if trade.Timestamp.Before(cursorAt) {
					continue
				}

				if trade.Timestamp.Equal(cursorAt) && trade.TradeID != 0 && trade.TradeID <= cursorID {
					continue
				}

				out = append(out, trade)

				if trade.Timestamp.After(latestAt) || (trade.Timestamp.Equal(latestAt) && trade.TradeID > latestID) {
					thesis.Measured.Store(source+"trades", tradeCursor{at: trade.Timestamp, tradeID: trade.TradeID})
					latestAt = trade.Timestamp
					latestID = trade.TradeID
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

func (thesis *Thesis) Fanout(sender SourceType) {
	thesis.subscribers.Range(func(key, value any) bool {
		source := key.(SourceType)

		if source == sender {
			return true
		}

		if sender != SourceTrader {
			switch source {
			case SourceCorrelation, SourceCVD, SourceDepthFlow, SourceExhaustion,
				SourceHawkes, SourceLeadLag, SourceLiquidity, SourcePumpDump,
				SourceSentiment, SourceToxicity:
				return true
			}
		}

		semaphore := value.(chan struct{})

		select {
		case <-thesis.ctx.Done():
			return false
		case semaphore <- struct{}{}:
		default:
		}

		return true
	})
}

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
