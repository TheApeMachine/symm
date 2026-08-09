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
	if len(measurements) == 0 {
		return nil
	}

	found, ok := thesis.Measurements.LoadOrStore(sender, measurements)

	if ok {
		stored := found.([]*Measurement)

		for _, newMeasurement := range measurements {
			replaced := false

			for index, measurement := range stored {
				if measurement.ID == newMeasurement.ID {
					return errnie.Error(errnie.Err(
						errnie.Conflict,
						fmt.Sprintf(
							"thesis: duplicate measurement found for [%s]",
							sender,
						),
						nil,
					))
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

	if ready {
		thesis.Readiness.Stamp(SourceType(measurements[0].Source))
		thesis.Fanout(sender)
	}

	return nil
}

/*
MarketTickers returns all the tickers in the thesis, except those that the
source has already seen. This is used to fan out new tickers to subscribers.
*/
func (thesis *Thesis) MarketTickers(source SourceType) []kraken.TickerData {
	out := make([]kraken.TickerData, 0)
	at := time.Time{}
	cursor := time.Time{}
	stored, ok := thesis.Measured.Load(source + "tickers")

	if ok {
		cursor = stored.(time.Time)
	}

	thesis.Tickers.Range(func(key, value any) bool {
		if tickerSlice, ok := value.([]kraken.TickerData); ok {
			for _, ticker := range tickerSlice {
				if !ticker.Timestamp.After(cursor) {
					continue
				}

				out = append(out, ticker)

				if ticker.Timestamp.After(at) {
					thesis.Measured.Store(source+"tickers", ticker.Timestamp)
					at = ticker.Timestamp
				}
			}
		}

		return true
	})

	return out
}

func (thesis *Thesis) MarketTrades(source SourceType) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)
	at := time.Time{}
	cursor := time.Time{}
	stored, ok := thesis.Measured.Load(source + "trades")

	if ok {
		cursor = stored.(time.Time)
	}

	thesis.Trades.Range(func(key, value any) bool {
		if tradeSlice, ok := value.([]kraken.TradeData); ok {
			for _, trade := range tradeSlice {
				if !trade.Timestamp.After(cursor) {
					continue
				}

				out = append(out, trade)

				if trade.Timestamp.After(at) {
					thesis.Measured.Store(source+"trades", trade.Timestamp)
					at = trade.Timestamp
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
