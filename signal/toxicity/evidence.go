package toxicity

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
symbolEvidence aggregates trade and fill evidence for one symbol so touch
honesty uses one event batch.
*/
type symbolEvidence struct {
	latestAt    time.Time
	tradeCount  int
	volume      float64
	fillBid     float64
	fillAsk     float64
	bidExecuted float64
	askExecuted float64
}

/*
observationContext carries the shared validity and scale contract for one
toxicity observation window anchored at a source event time.
*/
type observationContext struct {
	validity types.MeasurementValidity
	scale    types.ScaleReference
}

/*
newObservationContext builds validity from corroborating event count and scale
from the observation timestamp so Measure and touchHonesty share one contract.
*/
func newObservationContext(
	at time.Time,
	evidenceCount int,
) observationContext {
	return observationContext{
		validity: types.ObservationValidity(evidenceCount),
		scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    at,
			Through: at,
		},
	}
}

/*
resetIncrements clears and reloads price increments from the current books into
the Signal scratch map so Calculate does not allocate a fresh map each tick.
*/
func (signal *Signal) resetIncrements(frame *types.MarketFrame) {
	clear(signal.increments)

	if frame == nil {
		return
	}

	for _, row := range frame.Books {
		if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
			continue
		}

		signal.increments[row.Symbol] = row.PriceIncrement
	}
}

/*
resetEvidence drops last tick's per-symbol rows while retaining map capacity.
*/
func (signal *Signal) resetEvidence() {
	clear(signal.evidence)
}

/*
accumulateEvidence ingests validated trades and attributes touch fills per
symbol from the current Level3 book snapshot into Signal.evidence.
*/
func (signal *Signal) accumulateEvidence(trades []kraken.TradeData) {
	signal.resetEvidence()

	for _, trade := range trades {
		if trade.Price.Sign() <= 0 || trade.Qty <= 0 || trade.Timestamp.IsZero() {
			continue
		}

		at := trade.Timestamp.UTC()
		row := signal.evidence[trade.Symbol]

		if row == nil {
			row = &symbolEvidence{}
			signal.evidence[trade.Symbol] = row
		}

		row.tradeCount++
		row.volume += trade.Qty

		if at.After(row.latestAt) {
			row.latestAt = at
		}

		increment := signal.increments[trade.Symbol]

		signal.level3.PeekBook(trade.Symbol, func(symbolBook *book.Book) {
			if symbolBook.Name != trade.Symbol {
				return
			}

			bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

			attributeTouchFill(row, trade.Price, trade.Qty, bid, ask, increment)
		})
	}
}
