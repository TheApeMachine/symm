package hawkes

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures the buy/sell trade-arrival process as

	λ(t) = μ + Σ A exp(-β(t-ti)).

Measure always emits when asked: a counts observation on the first trade, a
fitted reading once the kernel is identifiable, or the last published reading
if nothing new has arrived. Maturity is closeness to a trustworthy fit.
hypothesis_separation is the category margin between buy and sell process
groups, not sample precision. Forecast readiness stays false until residual
and out-of-sample validation exists.
*/
type Signal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	api       *websocket.API
	process   *excitation.Process
	sample    *excitation.Sample
	lastTrade *sync.Map
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

/*
NewSignal constructs the excitation pipeline. Nomagique owns the symbol-local
arrival windows and fitted parameter epochs.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		api:       api,
		process:   excitation.NewProcess(),
		sample:    excitation.NewSample(),
		lastTrade: &sync.Map{},
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceHawkes
}

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return symbol.AlwaysYield(types.SourceHawkes, func(yield func(*types.Measurement) bool) {
		for trade := range symbol.MarketTrades(types.SourceHawkes) {
			if signal.seenTrade(trade) {
				continue
			}

			input, sampled, err := signal.sample.MeasureArrival(excitation.TradeInput{
				Symbol:    trade.Symbol,
				Side:      trade.Side,
				Timestamp: trade.Timestamp,
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"excitation sample failed: "+err.Error(),
					err,
				))

				continue
			}

			signal.commitTrade(trade)

			if !sampled {
				if !yield(signal.frame(trade.Symbol, countOutcome(trade, input))) {
					return
				}

				continue
			}

			outcome, measured, err := signal.process.Measure(input)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"excitation measure failed: "+err.Error(),
					err,
				))

				if !yield(signal.frame(trade.Symbol, countOutcome(trade, input))) {
					return
				}

				continue
			}

			if !measured {
				if !yield(signal.frame(trade.Symbol, countOutcome(trade, input))) {
					return
				}

				continue
			}

			if !yield(signal.frame(trade.Symbol, outcome)) {
				return
			}
		}
	})
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	if signal.lastTrade == nil {
		return false
	}

	raw, exists := signal.lastTrade.Load(row.Symbol)

	if !exists {
		return false
	}

	previous := raw.(tradeCursor)

	if row.Timestamp.Before(previous.at) {
		return true
	}

	if row.Timestamp.After(previous.at) {
		return false
	}

	_, seen := previous.ids[row.TradeID]

	return seen
}

func (signal *Signal) commitTrade(row kraken.TradeData) {
	if signal.lastTrade == nil {
		signal.lastTrade = &sync.Map{}
	}

	previous := tradeCursor{}
	raw, exists := signal.lastTrade.Load(row.Symbol)

	if exists {
		previous = raw.(tradeCursor)
	}

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade.Store(row.Symbol, previous)
}

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
