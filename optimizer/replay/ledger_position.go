package replay

import (
	"math"
	"strings"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
openLong sizes an entry from the fixed capital base and open-position cap, matching
trader/crypto.sizeEntry. Each slot deploys PositionFraction of StartingCapital, bounded
by remaining cash including fees, and at most round(1/fraction) positions may be open.
*/
func (ledger *replayLedger) openLong(
	symbol string,
	price float64,
	feePct float64,
	slippagePct float64,
	at time.Time,
) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked &&
		ticksSinceClose < ledger.reentryTickCooldown {
		return
	}

	if !ledger.fundableSymbol(symbol) {
		return
	}

	fraction := effectiveFraction(ledger.costs)
	capacity := int(math.Round(1 / fraction))

	if len(ledger.positions) >= capacity {
		ledger.fundBlocked++

		return
	}

	capital := effectiveCapital(ledger.costs)
	slot := fraction * capital

	if affordable := ledger.cash / (1 + feePct); slot > affordable {
		slot = affordable
	}

	if slot <= 0 {
		ledger.fundBlocked++

		return
	}

	entryFill := price * (1 + slippagePct)

	if entryFill <= 0 {
		return
	}

	ledger.observeSymbolPrice(symbol, entryFill)

	quantity := slot / (entryFill * (1 + feePct))

	if quantity <= 0 {
		return
	}

	ledger.cash -= slot

	ledger.positions[symbol] = replayPosition{
		entryPrice:  entryFill,
		quantity:    quantity,
		cost:        slot,
		peak:        entryFill,
		entryAt:     at,
		triggerType: perspectives.ActionNone,
	}

	delete(ledger.ticksSinceClose, symbol)
}

/*
fundableSymbol reports whether the wallet can pay for the pair: a buy spends the
pair's quote currency, so only pairs quoted in WalletCurrency are tradeable. An
empty WalletCurrency or an unparseable symbol falls back to fundable so plain
single-symbol replays are unaffected.
*/
func (ledger *replayLedger) fundableSymbol(symbol string) bool {
	if ledger.costs.WalletCurrency == "" {
		return true
	}

	slash := strings.LastIndex(symbol, "/")

	if slash < 0 {
		return true
	}

	return symbol[slash+1:] == ledger.costs.WalletCurrency
}

/*
settle closes a position back to cash and books the realized P&L in account
currency. exitFill is the per-unit fill price and feePct is the exit fee fraction.
*/
func (ledger *replayLedger) settle(symbol string, exitFill, feePct float64) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	netProceeds := position.quantity * exitFill * (1 - feePct)

	ledger.cash += netProceeds
	ledger.realized += netProceeds - position.cost
	ledger.closedTrades++
	delete(ledger.positions, symbol)
	ledger.ticksSinceClose[symbol] = 0
}

func (ledger *replayLedger) previewClosePnL(
	symbol string,
	price float64,
	spreadBPS float64,
) float64 {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 || price <= 0 {
		return 0
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, spreadBPS)
	exitFill := price * (1 - slippagePct)

	return (exitFill - position.entryPrice) / position.entryPrice
}

func (ledger *replayLedger) closeLong(
	symbol string,
	price float64,
	feePct float64,
	slippagePct float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	exitFill := price * (1 - slippagePct)

	ledger.settle(symbol, exitFill, feePct)
}

func (ledger *replayLedger) observations(
	symbol string,
) map[perspectives.ObservationType]float64 {
	clear(ledger.observationScratch)

	if ledger.holding(symbol) {
		ledger.observationScratch[perspectives.ObservationHolding] = 1

		return ledger.observationScratch
	}

	ledger.observationScratch[perspectives.ObservationNotHolding] = 1

	return ledger.observationScratch
}

func (ledger *replayLedger) metrics(
	measurement perspectives.Measurement,
) map[string]float64 {
	clear(ledger.metricsScratch)
	ledger.metricsScratch["last"] = measurement.Last

	position, open := ledger.positions[measurement.Symbol]

	if !open || position.entryPrice <= 0 || measurement.Last <= 0 {
		return ledger.metricsScratch
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, measurement.SpreadBPS)
	exitFill := measurement.Last * (1 - slippagePct)
	change := exitFill - position.entryPrice
	ledger.metricsScratch["unrealized_return"] = (change / position.entryPrice) * 100

	return ledger.metricsScratch
}

func (ledger *replayLedger) holding(symbol string) bool {
	_, open := ledger.positions[symbol]

	return open
}

/*
realizedReturn is the account's realized P&L as a fraction of starting capital,
so the score is a return on the real €200 — not a capital-free sum of per-trade
percentages that silently assumed unlimited money.
*/
func (ledger *replayLedger) realizedReturn() float64 {
	return ledger.realized / effectiveCapital(ledger.costs)
}

func HoldoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	return holdoutDecay(trainPerTrade, testPerTrade)
}

func holdoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	if trainPerTrade <= 0 {
		return math.Inf(1)
	}

	return (trainPerTrade - testPerTrade) / trainPerTrade
}
