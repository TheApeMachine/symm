package risk

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/wallet"
)

/*
Portfolio tracks price windows and peak equity for dampener calculations.
*/
type Portfolio struct {
	window   Window
	drawdown Drawdown
}

/*
NewPortfolio builds one portfolio risk tracker.
*/
func NewPortfolio() *Portfolio {
	return &Portfolio{
		window: *NewWindow(),
	}
}

/*
ObserveSymbol records one price tick at the current wall clock.
*/
func (portfolio *Portfolio) ObserveSymbol(symbol string, price float64) {
	portfolio.window.ObserveSymbol(symbol, price)
}

/*
ObserveSymbolAt records one price tick for grid-resampled correlation.
*/
func (portfolio *Portfolio) ObserveSymbolAt(symbol string, price float64, at time.Time) {
	portfolio.window.ObserveSymbolAt(symbol, price, at)
}

/*
Mark returns the latest observed price for one symbol.
*/
func (portfolio *Portfolio) Mark(symbol string) float64 {
	return portfolio.window.Mark(symbol)
}

/*
UpdatePeakEquity records the portfolio high-water mark for drawdown dampening.
*/
func (portfolio *Portfolio) UpdatePeakEquity(equity float64) {
	portfolio.drawdown.UpdatePeakEquity(equity)
}

/*
Adjust computes the dampener for one candidate entry against open exposure.
*/
func (portfolio *Portfolio) Adjust(
	tradingWallet *wallet.Wallet,
	measurement engine.Measurement,
	openSymbols []string,
) Adjuster {
	return (&Adjuster{}).For(portfolio, tradingWallet, measurement, openSymbols)
}

/*
DrawdownPct reports peak-to-current equity drawdown as a unit fraction.
*/
func (portfolio *Portfolio) DrawdownPct(tradingWallet *wallet.Wallet) float64 {
	return portfolio.drawdown.Pct(tradingWallet, portfolio.window.Marks())
}

/*
SystemicCorrelation measures portfolio concentration via the correlation spectrum.
*/
func (portfolio *Portfolio) SystemicCorrelation(
	candidate string,
	openSymbols []string,
) (float64, bool) {
	symbols := portfolio.candidateSymbols(candidate, openSymbols)

	if len(symbols) < 2 || config.System.MaxSymbolCorrelation <= 0 {
		return 0, false
	}

	matrix, ok := portfolio.window.BuildMatrix(symbols)

	if !ok {
		return 0, false
	}

	return matrix.SystemicConcentration()
}

/*
MarketRegime classifies recent price path efficiency for one symbol.
*/
func (portfolio *Portfolio) MarketRegime(symbol string) engine.MarketRegime {
	return portfolio.window.MarketRegime(symbol)
}

func (portfolio *Portfolio) candidateSymbols(candidate string, openSymbols []string) []string {
	seen := make(map[string]struct{}, len(openSymbols)+1)
	symbols := make([]string, 0, len(openSymbols)+1)

	for _, symbol := range openSymbols {
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	if candidate == "" {
		return symbols
	}

	if _, ok := seen[candidate]; ok {
		return symbols
	}

	return append(symbols, candidate)
}
