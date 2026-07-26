package toxicity

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
)

/*
symbolEvidence aggregates trade and fill evidence for one symbol so touch
honesty uses one event batch.
*/
type symbolEvidence struct {
	latestAt    time.Time
	tradeCount  int
	bookCount   int
	volume      float64
	fillBid     float64
	fillAsk     float64
	bidExecuted float64
	askExecuted float64
}

/*
ingestIncrements retains each symbol's observed price lattice and rejects a
malformed book rather than silently falling back to decimal price proximity.
*/
func (signal *Signal) ingestIncrements(books []kraken.BookData) error {
	for _, row := range books {
		if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
			continue
		}

		signal.increments[row.Symbol] = row.PriceIncrement
	}

	return nil
}

/*
resetEvidence drops last tick's per-symbol rows while retaining map capacity.
*/
func (signal *Signal) resetEvidence() {
	clear(signal.evidence)
}

/*
accumulateEvidence ingests validated trades and attributes touch fills per
symbol from the current Level3 book snapshot into Signal.evidence. Trade and
book cuts that share one market timestamp keep the same evidence bag so a
book publish after a same-instant trade still emits trade volume.
*/
func (signal *Signal) accumulateEvidence(
	trades []kraken.TradeData, cutAt time.Time,
) error {
	if signal.lastCutAt.IsZero() || cutAt.After(signal.lastCutAt) {
		signal.resetEvidence()
		signal.lastCutAt = cutAt
	}

	for _, trade := range trades {
		if trade.Symbol == "" || trade.Price.Sign() <= 0 || trade.Qty <= 0 ||
			trade.Timestamp.IsZero() || trade.Side != "buy" && trade.Side != "sell" {
			continue
		}

		increment := signal.increments[trade.Symbol]

		if increment == nil || increment.Sign() <= 0 {
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

		prior, observed := signal.priorTouch[trade.Symbol]

		if observed {
			if err := attributeTouchPrices(
				row,
				trade.Price,
				trade.Qty,
				&prior.bidPrice,
				&prior.askPrice,
			); err != nil {
				continue
			}

			continue
		}

		attributed := false
		var err error
		peeked := signal.level3.PeekBook(trade.Symbol, func(symbolBook *book.Book) {
			if symbolBook.Name != trade.Symbol {
				return
			}

			bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()
			err = attributeTouchFill(row, trade.Price, trade.Qty, bid, ask)
			attributed = err == nil
		})

		if err != nil || !peeked || !attributed {
			continue
		}
	}

	return nil
}

/*
observeBooks creates evidence rows for symbols whose Level3 touch changed
without a public trade. This lets cancellation and retreat remain observable
without fabricating an unrelated execution to trigger the signal.
*/
func (signal *Signal) observeBooks(books []kraken.BookData) error {
	for _, bookRow := range books {
		if bookRow.Symbol == "" || bookRow.Timestamp.IsZero() {
			continue
		}

		row := signal.evidence[bookRow.Symbol]

		if row == nil {
			row = &symbolEvidence{}
			signal.evidence[bookRow.Symbol] = row
		}

		row.bookCount++
		at := bookRow.Timestamp.UTC()

		if at.After(row.latestAt) {
			row.latestAt = at
		}
	}

	return nil
}
