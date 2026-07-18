package toxicity

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
	volume      *decimal.Decimal
	fillBid     *decimal.Decimal
	fillAsk     *decimal.Decimal
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
priceIncrementsBySymbol indexes each symbol's live price increment from the
current market frame so touch attribution uses exchange structure.
*/
func priceIncrementsBySymbol(frame *types.MarketFrame) map[string]*decimal.Decimal {
	incrementBySymbol := map[string]*decimal.Decimal{}

	for _, row := range frame.Books {
		if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
			continue
		}

		incrementBySymbol[row.Symbol] = row.PriceIncrement
	}

	return incrementBySymbol
}

/*
accumulateEvidence ingests validated trades and attributes touch fills per
symbol from the current Level3 book snapshot.
*/
func (signal *Signal) accumulateEvidence(
	trades []kraken.TradeData,
	incrementBySymbol map[string]*decimal.Decimal,
) map[string]*symbolEvidence {
	evidence := map[string]*symbolEvidence{}

	for _, trade := range trades {
		if trade.Price.Sign() <= 0 || trade.Qty <= 0 || trade.Timestamp.IsZero() {
			continue
		}

		at := trade.Timestamp.UTC()
		row := evidence[trade.Symbol]

		if row == nil {
			row = &symbolEvidence{}
			evidence[trade.Symbol] = row
		}

		row.tradeCount++
		volume := decimal.NewFromFloat64(trade.Qty)
		row.volume = zeroed(row.volume).Add(volume)

		if at.After(row.latestAt) {
			row.latestAt = at
		}

		increment := incrementBySymbol[trade.Symbol]

		signal.level3.PeekBook(trade.Symbol, func(symbolBook *book.Book) {
			if symbolBook.Name != trade.Symbol {
				return
			}

			bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

			attributeTouchFill(row, trade.Price, trade.Qty, bid, ask, increment)
		})
	}

	return evidence
}

/*
zeroed returns total, or a fresh zero accumulator when total has not been
seeded yet for this tick.
*/
func zeroed(total *decimal.Decimal) *decimal.Decimal {
	if total == nil {
		return decimal.NewFromFloat64(0)
	}

	return total
}
