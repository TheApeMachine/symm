package replay

import (
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

type pendingMakerEntry struct {
	symbol     string
	side       trading.Side
	limitPrice float64
	quantity   float64
	feePct     float64
	at         types.Measurement
	queue      broker.MakerQueueState
	tickSize   float64
}

func (ledger *replayLedger) queueMakerEntry(
	symbol string,
	side trading.Side,
	limitPrice, quantity, feePct float64,
	fraction float64,
	measurement types.Measurement,
) {
	if !ledger.canReserveEntry(fraction, 0, quoteCurrency(symbol)) {
		ledger.fundBlocked++

		return
	}

	if !measurement.HasBookDepth() {
		ledger.preflightBlocked++

		return
	}

	if ledger.instrumentRules == nil {
		ledger.preflightBlocked++

		return
	}

	tickSize, err := ledger.instrumentRules.PriceTickSize(symbol)

	if err != nil {
		ledger.preflightBlocked++

		return
	}

	quote := broker.QuoteFromMeasurement(measurement)
	activeAt := measurement.At.UnixNano()

	ledger.pendingMakers = append(ledger.pendingMakers, pendingMakerEntry{
		symbol:     symbol,
		side:       side,
		limitPrice: limitPrice,
		quantity:   quantity,
		feePct:     feePct,
		at:         measurement,
		queue: broker.NewMakerQueueState(
			quote,
			side,
			limitPrice,
			activeAt,
			tickSize,
		),
		tickSize: tickSize,
	})
}

func (ledger *replayLedger) advanceMakerQueues(row types.Measurement) {
	if len(ledger.pendingMakers) == 0 || row.Symbol == "" || row.Last <= 0 {
		return
	}

	if !row.HasBookDepth() {
		return
	}

	previousRow, hasPrevious := ledger.priorQuotedRows[row.Symbol]
	currentQuote := broker.QuoteFromMeasurement(row)
	remaining := ledger.pendingMakers[:0]

	for _, pending := range ledger.pendingMakers {
		if pending.symbol != row.Symbol {
			remaining = append(remaining, pending)
			continue
		}

		if row.At.UnixNano() < pending.queue.ActiveAt {
			remaining = append(remaining, pending)
			continue
		}

		if !makerPriceReachable(pending.side, pending.limitPrice, row.Last) {
			remaining = append(remaining, pending)
			continue
		}

		if hasPrevious && previousRow.HasBookDepth() {
			previousQuote := broker.QuoteFromMeasurement(previousRow)
			depletion := broker.MakerBookDepletion(
				pending.side,
				pending.limitPrice,
				previousQuote,
				currentQuote,
				pending.tickSize,
			)
			pending.queue.Deplete(depletion)
		}

		if !pending.queue.Ready() {
			remaining = append(remaining, pending)
			continue
		}

		trade := makerLiftTrade(pending.side, pending.limitPrice, row.Last, pending.quantity)
		fillPrice, _ := broker.MakerRestingFillPrice(
			pending.side,
			pending.limitPrice,
			currentQuote,
			trade,
			pending.tickSize,
		)

		if fillPrice <= 0 {
			remaining = append(remaining, pending)
			continue
		}

		fillRow := row
		fillRow.Last = fillPrice

		ledger.openEntryReserved(
			pending.symbol,
			pending.side,
			reasoning.Act{Type: reasoning.ActionLimit, Side: pending.side},
			fillRow,
			nil,
			pending.feePct,
			row.At,
			1,
		)
	}

	ledger.pendingMakers = remaining
}

func makerLiftTrade(
	side trading.Side,
	limitPrice float64,
	lastPrice float64,
	quantity float64,
) market.TradeUpdate {
	trade := market.TradeUpdate{
		Price: limitPrice,
		Qty:   quantity,
	}

	if lastPrice > 0 {
		trade.Price = lastPrice
	}

	if side == trading.Buy {
		trade.Side = "sell"

		return trade
	}

	trade.Side = "buy"

	return trade
}

func makerPriceReachable(side trading.Side, limitPrice, marketPrice float64) bool {
	if limitPrice <= 0 || marketPrice <= 0 {
		return false
	}

	if side == trading.Sell {
		return marketPrice >= limitPrice
	}

	return marketPrice <= limitPrice
}
