package trader

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"gonum.org/v1/gonum/mat"
)

/*
RiskAdjustment is the position-scale dampener computed for one candidate entry.
*/
type RiskAdjustment struct {
	Dampener            float64
	Reason              string
	DrawdownPct         float64
	SystemicCorrelation float64
	HasSystemicMeasure  bool
}

/*
Risk tracks price windows and portfolio peak equity for dampener calculations.
*/
type Risk struct {
	peakEquity float64
	lastPrices map[string]float64
	prices     map[string]priceSampleRing
	windowCap  int
	minSamples int
}

/*
NewRisk builds rolling return windows sized from config.
*/
func NewRisk() *Risk {
	windowCap := config.System.MinCorrelationSamples

	if config.System.PriceHistory > 0 && config.System.PriceHistory < windowCap {
		windowCap = config.System.PriceHistory
	}

	return &Risk{
		lastPrices: make(map[string]float64),
		prices:     make(map[string]priceSampleRing),
		windowCap:  windowCap,
		minSamples: config.System.MinCorrelationSamples,
	}
}

/*
ObserveSymbol records one price tick at the current wall clock.
*/
func (risk *Risk) ObserveSymbol(symbol string, price float64) {
	risk.ObserveSymbolAt(symbol, price, time.Now())
}

/*
ObserveSymbolAt records one price tick for grid-resampled correlation.
*/
func (risk *Risk) ObserveSymbolAt(symbol string, price float64, at time.Time) {
	if risk == nil || symbol == "" || price <= 0 {
		return
	}

	if at.IsZero() {
		at = time.Now()
	}

	sampleWindow, ok := risk.prices[symbol]

	if !ok {
		sampleWindow = newPriceSampleRing(risk.windowCap)
	}

	sampleWindow.push(at, price)
	risk.prices[symbol] = sampleWindow
	risk.lastPrices[symbol] = price
}

/*
Mark returns the latest observed price for one symbol.
*/
func (risk *Risk) Mark(symbol string) float64 {
	if risk == nil {
		return 0
	}

	return risk.lastPrices[symbol]
}

/*
UpdatePeakEquity records the portfolio high-water mark for drawdown dampening.
*/
func (risk *Risk) UpdatePeakEquity(equity float64) {
	if risk == nil || equity <= 0 {
		return
	}

	if risk.peakEquity <= 0 || equity > risk.peakEquity {
		risk.peakEquity = equity
	}
}

/*
Adjustment computes the dampener for one candidate entry against open exposure.
*/
func (risk *Risk) Adjustment(
	wallet *Wallet,
	measurement engine.Measurement,
	openSymbols []string,
) RiskAdjustment {
	adjustment := RiskAdjustment{Dampener: 1}

	if risk == nil || wallet == nil || len(measurement.Pairs) == 0 {
		return adjustment
	}

	symbol := measurement.Pairs[0].Wsname
	adjustment.DrawdownPct = risk.DrawdownPct(wallet)
	adjustment.applyDrawdownDampener()

	systemicCorrelation, ok := risk.SystemicCorrelation(symbol, openSymbols)

	if !ok {
		return adjustment
	}

	adjustment.SystemicCorrelation = systemicCorrelation
	adjustment.HasSystemicMeasure = true
	adjustment.applySystemicDampener()

	return adjustment
}

/*
DrawdownPct reports peak-to-current equity drawdown as a unit fraction.
*/
func (risk *Risk) DrawdownPct(wallet *Wallet) float64 {
	if risk == nil || wallet == nil || risk.peakEquity <= 0 {
		return 0
	}

	equity := wallet.MarkEquity(risk.lastPrices)

	if equity <= 0 || equity >= risk.peakEquity {
		return 0
	}

	return (risk.peakEquity - equity) / risk.peakEquity
}

/*
SystemicCorrelation measures portfolio concentration via the correlation spectrum.
*/
func (risk *Risk) SystemicCorrelation(
	candidate string,
	openSymbols []string,
) (float64, bool) {
	symbols := candidatePortfolioSymbols(candidate, openSymbols)

	if len(symbols) < 2 || config.System.MaxSymbolCorrelation <= 0 {
		return 0, false
	}

	matrix, ok := risk.correlationMatrix(symbols)

	if !ok {
		return 0, false
	}

	eigenvalue, ok := principalEigenvalue(matrix)

	if !ok {
		return 0, false
	}

	concentration := (eigenvalue - 1) / (float64(len(symbols)) - 1)

	return clampUnit(concentration), true
}

/*
MarketRegime classifies recent price path efficiency for one symbol.
*/
func (risk *Risk) MarketRegime(symbol string) engine.MarketRegime {
	if risk == nil || symbol == "" {
		return engine.RegimeUnknown
	}

	samples := risk.prices[symbol].ordered()

	if len(samples) < risk.minSamples+1 {
		return engine.RegimeUnknown
	}

	return regimeFromReturns(logReturnsFromPrices(samplePrices(samples)))
}

func (risk *Risk) correlationMatrix(symbols []string) ([][]float64, bool) {
	matrix := make([][]float64, len(symbols))

	for row := range matrix {
		matrix[row] = make([]float64, len(symbols))
		matrix[row][row] = 1
	}

	for row := 0; row < len(symbols); row++ {
		for col := row + 1; col < len(symbols); col++ {
			correlation, ok := risk.symbolCorrelation(symbols[row], symbols[col])

			if !ok {
				return nil, false
			}

			correlation = math.Max(correlation, 0)
			matrix[row][col] = correlation
			matrix[col][row] = correlation
		}
	}

	return matrix, true
}

func (risk *Risk) symbolCorrelation(left, right string) (float64, bool) {
	leftSamples := risk.prices[left].ordered()
	rightSamples := risk.prices[right].ordered()

	if len(leftSamples) < 2 || len(rightSamples) < 2 {
		return 0, false
	}

	leftReturns, rightReturns, ok := synchronizedLogReturns(
		leftSamples, rightSamples, correlationBarInterval(),
	)

	if !ok || len(leftReturns) < risk.minSamples {
		return 0, false
	}

	return pearson(leftReturns, rightReturns), true
}

func (adjustment *RiskAdjustment) applyDrawdownDampener() {
	limit := config.System.MaxPortfolioDrawdownPct

	if limit <= 0 || adjustment.DrawdownPct <= 0 {
		return
	}

	remaining := 1 - adjustment.DrawdownPct/limit
	adjustment.applyDampener(clampUnit(remaining), "drawdown_dampened")
}

func (adjustment *RiskAdjustment) applySystemicDampener() {
	limit := config.System.MaxSymbolCorrelation

	if limit <= 0 || adjustment.SystemicCorrelation <= 0 {
		return
	}

	remaining := 1 - adjustment.SystemicCorrelation/limit
	adjustment.applyDampener(clampUnit(remaining), "covariance_dampened")
}

func (adjustment *RiskAdjustment) applyDampener(candidate float64, reason string) {
	if candidate >= adjustment.Dampener {
		return
	}

	adjustment.Dampener = candidate
	adjustment.Reason = reason
}

func principalEigenvalue(matrix [][]float64) (float64, bool) {
	if len(matrix) == 0 {
		return 0, false
	}

	size := len(matrix)
	data := make([]float64, size*size)

	for row := range matrix {
		if len(matrix[row]) != size {
			return 0, false
		}

		for col := range matrix[row] {
			data[row*size+col] = matrix[row][col]
		}
	}

	var eigen mat.EigenSym

	if !eigen.Factorize(mat.NewSymDense(size, data), false) {
		return 0, false
	}

	values := eigen.Values(nil)
	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak, true
}

func regimeFromReturns(returns []float64) engine.MarketRegime {
	if len(returns) == 0 {
		return engine.RegimeUnknown
	}

	netReturn := 0.0
	pathLength := 0.0

	for _, value := range returns {
		netReturn += value
		pathLength += math.Abs(value)
	}

	if pathLength <= 0 {
		return engine.RegimeDead
	}

	efficiency := math.Abs(netReturn) / pathLength
	noiseFloor := 1 / math.Sqrt(float64(len(returns)))

	if efficiency <= noiseFloor {
		return engine.RegimeChoppy
	}

	if netReturn > 0 {
		return engine.RegimeBullish
	}

	if netReturn < 0 {
		return engine.RegimeBearish
	}

	return engine.RegimeTrending
}

func candidatePortfolioSymbols(candidate string, openSymbols []string) []string {
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

func samplePrices(samples []priceSample) []float64 {
	prices := make([]float64, 0, len(samples))

	for _, sample := range samples {
		if sample.price <= 0 {
			continue
		}

		prices = append(prices, sample.price)
	}

	return prices
}

func clampUnit(value float64) float64 {
	if value <= 0 {
		return 0
	}

	if value >= 1 {
		return 1
	}

	return value
}
