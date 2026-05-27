package trader

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
PortfolioRisk tracks equity, return windows, and enforces portfolio entry gates.
*/
type PortfolioRisk struct {
	peakEquity     float64
	dayStartEquity float64
	dayAnchor      time.Time
	lastPrices     map[string]float64
	prices         map[string]priceSampleRing
	windowCap      int
	minSamples     int
}

/*
NewPortfolioRisk builds rolling return windows sized from config.
*/
func NewPortfolioRisk() *PortfolioRisk {
	windowCap := config.System.MinCorrelationSamples

	if config.System.PriceHistory > 0 && config.System.PriceHistory < windowCap {
		windowCap = config.System.PriceHistory
	}

	return &PortfolioRisk{
		lastPrices: make(map[string]float64),
		prices:     make(map[string]priceSampleRing),
		windowCap:  windowCap,
		minSamples: config.System.MinCorrelationSamples,
	}
}

/*
ObserveSymbol records one price tick at the current wall clock.
*/
func (portfolioRisk *PortfolioRisk) ObserveSymbol(symbol string, price float64) {
	portfolioRisk.ObserveSymbolAt(symbol, price, time.Now())
}

/*
ObserveSymbolAt records one price tick for grid-resampled correlation.
*/
func (portfolioRisk *PortfolioRisk) ObserveSymbolAt(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 {
		return
	}

	if at.IsZero() {
		at = time.Now()
	}

	sampleWindow, ok := portfolioRisk.prices[symbol]

	if !ok {
		sampleWindow = newPriceSampleRing(portfolioRisk.windowCap)
	}

	sampleWindow.push(at, price)
	portfolioRisk.prices[symbol] = sampleWindow
	portfolioRisk.lastPrices[symbol] = price
}

/*
Mark returns the latest observed price for one symbol.
*/
func (portfolioRisk *PortfolioRisk) Mark(symbol string) float64 {
	if portfolioRisk == nil {
		return 0
	}

	return portfolioRisk.lastPrices[symbol]
}

/*
UpdateEquity anchors daily PnL and peak equity for drawdown checks.
*/
func (portfolioRisk *PortfolioRisk) UpdateEquity(equity float64, now time.Time) {
	if equity <= 0 {
		return
	}

	if portfolioRisk.dayAnchor.IsZero() || !sameUTCDate(portfolioRisk.dayAnchor, now) {
		portfolioRisk.dayAnchor = startOfUTCDate(now)
		portfolioRisk.dayStartEquity = equity
	}

	if portfolioRisk.peakEquity <= 0 || equity > portfolioRisk.peakEquity {
		portfolioRisk.peakEquity = equity
	}
}

/*
AllowEntry reports whether a candidate entry satisfies portfolio risk limits.
*/
func (portfolioRisk *PortfolioRisk) AllowEntry(
	wallet *Wallet,
	measurement engine.Measurement,
	slot float64,
	openSymbols []string,
) (bool, string) {
	if wallet == nil || len(measurement.Pairs) == 0 {
		return false, "wallet_or_symbol_missing"
	}

	symbol := measurement.Pairs[0].Wsname
	last := anchorPrice(measurement)
	bid := measurement.Bid
	ask := measurement.Ask

	if last <= 0 {
		return false, "missing_price"
	}

	if bid <= 0 {
		bid = last
	}

	if ask <= 0 {
		ask = last
	}

	if slot < config.System.MinCostEUR {
		return false, "slot_below_min_cost"
	}

	if slot > wallet.AvailableEUR() {
		return false, "insufficient_margin"
	}

	maxSlotFromLoss := maxSlotNotional(config.System.MaxLossPerTradeEUR)

	if maxSlotFromLoss > 0 && slot > maxSlotFromLoss {
		return false, "per_trade_loss_cap"
	}

	spreadBPS := quoteSpreadBPS(last, bid, ask)

	if config.System.MaxSpreadBPS > 0 && spreadBPS > config.System.MaxSpreadBPS {
		return false, fmt.Sprintf("spread_bps:%.2f", spreadBPS)
	}

	fillPrice := config.System.SlippageFill(
		last, bid, ask, "buy", config.System.SlippageBPS, slot, nil, nil,
	)
	mid := quoteMid(last, bid, ask)
	entrySlippageBPS := buySlippageBPS(fillPrice, mid)

	if config.System.MaxEntrySlippageBPS > 0 && entrySlippageBPS > config.System.MaxEntrySlippageBPS {
		return false, fmt.Sprintf("entry_slippage_bps:%.2f", entrySlippageBPS)
	}

	equity := wallet.MarkEquity(portfolioRisk.lastPrices)

	if equity <= 0 {
		return false, "equity_unpriced"
	}

	dailyLoss := portfolioRisk.dayStartEquity - equity

	if config.System.MaxDailyLossEUR > 0 && dailyLoss >= config.System.MaxDailyLossEUR {
		return false, "daily_loss_limit"
	}

	if config.System.MaxPortfolioDrawdownPct > 0 && portfolioRisk.peakEquity > 0 {
		drawdown := (portfolioRisk.peakEquity - equity) / portfolioRisk.peakEquity

		if drawdown >= config.System.MaxPortfolioDrawdownPct {
			return false, "drawdown_limit"
		}
	}

	deployed := equity - wallet.AvailableEUR()
	projectedDeploy := deployed + slot
	maxDeploy := equity * config.System.MaxDeployPct / 100

	if config.System.MaxDeployPct > 0 && projectedDeploy > maxDeploy {
		return false, "deploy_limit"
	}

	if correlatedSlotCount(portfolioRisk, symbol, openSymbols) >= config.System.MaxCorrelatedSlots {
		return false, "correlation_limit"
	}

	return true, ""
}

func correlatedSlotCount(
	portfolioRisk *PortfolioRisk,
	candidate string,
	openSymbols []string,
) int {
	if config.System.MaxSymbolCorrelation <= 0 {
		return 0
	}

	correlated := 0

	for _, openSymbol := range openSymbols {
		if openSymbol == candidate {
			continue
		}

		correlation, ok := portfolioRisk.symbolCorrelation(candidate, openSymbol)

		if !ok {
			continue
		}

		if correlation >= config.System.MaxSymbolCorrelation {
			correlated++
		}
	}

	return correlated
}

func (portfolioRisk *PortfolioRisk) symbolCorrelation(left, right string) (float64, bool) {
	leftSamples := portfolioRisk.prices[left].ordered()
	rightSamples := portfolioRisk.prices[right].ordered()

	if len(leftSamples) < 2 || len(rightSamples) < 2 {
		return 0, false
	}

	leftReturns, rightReturns, ok := synchronizedLogReturns(
		leftSamples, rightSamples, correlationBarInterval(),
	)

	if !ok || len(leftReturns) < portfolioRisk.minSamples {
		return 0, false
	}

	return pearson(leftReturns, rightReturns), true
}

func pearson(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}

	leftMean := 0.0
	rightMean := 0.0

	for index := range left {
		leftMean += left[index]
		rightMean += right[index]
	}

	sampleCount := float64(len(left))
	leftMean /= sampleCount
	rightMean /= sampleCount

	covariance := 0.0
	leftVariance := 0.0
	rightVariance := 0.0

	for index := range left {
		leftDelta := left[index] - leftMean
		rightDelta := right[index] - rightMean
		covariance += leftDelta * rightDelta
		leftVariance += leftDelta * leftDelta
		rightVariance += rightDelta * rightDelta
	}

	if leftVariance <= 0 || rightVariance <= 0 {
		return 0
	}

	return covariance / math.Sqrt(leftVariance*rightVariance)
}

func maxSlotNotional(maxLossEUR float64) float64 {
	if maxLossEUR <= 0 || config.System.DefaultTrailPct <= 0 {
		return 0
	}

	return maxLossEUR / (config.System.DefaultTrailPct / 100)
}

func quoteMid(last, bid, ask float64) float64 {
	if bid > 0 && ask > 0 {
		return (bid + ask) / 2
	}

	return last
}

func quoteSpreadBPS(last, bid, ask float64) float64 {
	mid := quoteMid(last, bid, ask)

	if mid <= 0 || bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	return (ask - bid) / mid * 10000
}

func buySlippageBPS(fillPrice, mid float64) float64 {
	if fillPrice <= 0 || mid <= 0 {
		return math.MaxFloat64
	}

	return (fillPrice - mid) / mid * 10000
}

func startOfUTCDate(now time.Time) time.Time {
	utc := now.UTC()

	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func sameUTCDate(left, right time.Time) bool {
	leftUTC := left.UTC()
	rightUTC := right.UTC()

	return leftUTC.Year() == rightUTC.Year() &&
		leftUTC.Month() == rightUTC.Month() &&
		leftUTC.Day() == rightUTC.Day()
}

func openSymbols(wallet *Wallet) []string {
	if wallet == nil {
		return nil
	}

	symbols := make([]string, 0, len(wallet.Inventory))

	for base, qty := range wallet.Inventory {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		symbols = append(symbols, base+"/"+wallet.Currency)
	}

	return symbols
}

func observeBatch(portfolioRisk *PortfolioRisk, batch []engine.Measurement, now time.Time) {
	for _, measurement := range batch {
		if len(measurement.Pairs) == 0 {
			continue
		}

		price := anchorPrice(measurement)

		if price <= 0 {
			continue
		}

		at := now

		if measurement.Timeframe.End > 0 {
			at = time.Unix(0, measurement.Timeframe.End)
		}

		portfolioRisk.ObserveSymbolAt(measurement.Pairs[0].Wsname, price, at)
	}
}
