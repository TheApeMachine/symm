package replay

import (
	"math"
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func quoteCurrency(symbol string) string {
	slash := strings.LastIndex(symbol, "/")

	if slash < 0 {
		return ""
	}

	return symbol[slash+1:]
}

func (ledger *replayLedger) walletCash(currency string) float64 {
	if ledger.cash == nil {
		return 0
	}

	return ledger.cash[currency]
}

func (ledger *replayLedger) debitWallet(currency string, amount float64) bool {
	if amount <= 0 {
		return true
	}

	if ledger.walletCash(currency) < amount {
		return false
	}

	ledger.cash[currency] -= amount

	return true
}

func (ledger *replayLedger) creditWallet(currency string, amount float64) {
	if amount <= 0 {
		return
	}

	ledger.cash[currency] += amount
}

/*
openEntry sizes and opens a long or short from the quote wallet and open-position cap.
When the tape carries L2 depth, broker.SlippageFill supplies the VWAP; insufficient
depth books a penalty and refuses the entry so MCTS cannot hallucinate liquidity.
*/
func (ledger *replayLedger) openEntry(
	symbol string,
	side trading.Side,
	act reasoning.Act,
	measurement types.Measurement,
	snapshots []types.Measurement,
	feePct float64,
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

	fraction := entryDeployFraction(ledger.costs, act, snapshots)
	capacity := int(math.Round(1 / fraction))

	if len(ledger.positions) >= capacity {
		ledger.fundBlocked++

		return
	}

	quote := quoteCurrency(symbol)
	capital := ledger.costs.WalletBalance(quote)
	slot := fraction * capital

	if affordable := ledger.walletCash(quote) / (1 + feePct); slot > affordable {
		slot = affordable
	}

	if slot <= 0 {
		ledger.fundBlocked++

		return
	}

	quantity := slot / measurement.Last

	if quantity <= 0 {
		return
	}

	fill, entryFill, filled := ledger.resolveEntryFill(side, measurement, snapshots, quantity)

	if !filled {
		return
	}

	if measurement.HasBookDepth() && fill.depthCoverage < minDepthCoverage() {
		ledger.depthBlocked++
		ledger.realized -= depthShortfallPenalty(
			slot, fill.depthCoverage, ledger.costs, measurement, snapshots,
		)

		return
	}

	ledger.observeSymbolPrice(symbol, entryFill)

	quantity = slot / entryFill

	if quantity <= 0 {
		return
	}

	if !ledger.debitWallet(quote, slot) {
		ledger.fundBlocked++

		return
	}

	position := replayPosition{
		side:        side,
		entryPrice:  entryFill,
		quantity:    quantity,
		cost:        slot,
		entryAt:     at,
		triggerType: reasoning.ActionNone,
	}

	if side == trading.Sell {
		position.trough = entryFill
	} else {
		position.peak = entryFill
	}

	ledger.positions[symbol] = position
	delete(ledger.ticksSinceClose, symbol)
}

func (ledger *replayLedger) resolveEntryFill(
	side trading.Side,
	measurement types.Measurement,
	snapshots []types.Measurement,
	quantity float64,
) (executionFill, float64, bool) {
	if measurement.HasBookDepth() {
		fill, err := takerFill(ledger.costs, measurement, side, quantity, snapshots)

		if err != nil {
			return executionFill{}, 0, false
		}

		return fill, fill.price, true
	}

	slippagePct := flatSlippagePct(ledger.costs, measurement.SpreadBPS, snapshots)

	return executionFill{slippagePct: slippagePct}, fillPriceFromPct(side, measurement.Last, slippagePct), true
}

/*
fundableSymbol reports whether the wallet can pay for the pair's quote currency.
*/
func (ledger *replayLedger) fundableSymbol(symbol string) bool {
	quote := quoteCurrency(symbol)

	if quote == "" {
		return true
	}

	return ledger.costs.WalletBalance(quote) > 0 || ledger.walletCash(quote) > 0
}

/*
settle closes a position back to cash and books realized P&L in the quote currency.
*/
func (ledger *replayLedger) settle(symbol string, exitFill, feePct float64) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	quote := quoteCurrency(symbol)

	if position.side == trading.Sell {
		exitCost := position.quantity * exitFill * (1 + feePct)
		entryProceeds := position.quantity * position.entryPrice * (1 - feePct)
		ledger.realized += entryProceeds - exitCost
		ledger.creditWallet(quote, position.cost+entryProceeds-exitCost)
	} else {
		netProceeds := position.quantity * exitFill * (1 - feePct)
		ledger.creditWallet(quote, netProceeds)
		ledger.realized += netProceeds - position.cost
	}

	ledger.closedTrades++
	delete(ledger.positions, symbol)
	ledger.ticksSinceClose[symbol] = 0
}

func (ledger *replayLedger) closePosition(
	symbol string,
	measurement types.Measurement,
	snapshots []types.Measurement,
	feePct float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	exitFill := ledger.resolveExitFill(position.side, measurement, snapshots, position.quantity)
	ledger.settle(symbol, exitFill, feePct)
}

func (ledger *replayLedger) resolveExitFill(
	side trading.Side,
	measurement types.Measurement,
	snapshots []types.Measurement,
	quantity float64,
) float64 {
	if measurement.HasBookDepth() && quantity > 0 {
		fill, err := takerFill(ledger.costs, measurement, side, quantity, snapshots)

		if err == nil {
			return fill.price
		}
	}

	slippagePct := flatSlippagePct(ledger.costs, measurement.SpreadBPS, snapshots)

	return fillPriceFromPct(side, measurement.Last, slippagePct)
}

func (ledger *replayLedger) holding(symbol string) bool {
	_, open := ledger.positions[symbol]

	return open
}

/*
realizedReturn is the account's realized P&L as a fraction of total starting capital.
*/
func (ledger *replayLedger) realizedReturn() float64 {
	totalCapital := 0.0

	for _, balance := range ledger.costs.WalletBalances {
		totalCapital += balance
	}

	if totalCapital <= 0 {
		totalCapital = effectiveCapital(ledger.costs)
	}

	return ledger.realized / totalCapital
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

func (ledger *replayLedger) openLong(
	symbol string,
	price float64,
	feePct float64,
	at time.Time,
) {
	ledger.openEntry(
		symbol,
		trading.Buy,
		reasoning.Act{},
		types.Measurement{Symbol: symbol, Last: price},
		nil,
		feePct,
		at,
	)
}

func (ledger *replayLedger) openShort(
	symbol string,
	price float64,
	feePct float64,
	at time.Time,
) {
	ledger.openEntry(
		symbol,
		trading.Sell,
		reasoning.Act{},
		types.Measurement{Symbol: symbol, Last: price},
		nil,
		feePct,
		at,
	)
}
