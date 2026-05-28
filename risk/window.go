package risk

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/correlation"
	"github.com/theapemachine/symm/engine"
)

/*
Window stores rolling price samples for correlation and regime classification.
*/
type Window struct {
	lastPrices map[string]float64
	prices     map[string]correlation.PriceSampleRing
	windowCap  int
	minSamples int
}

/*
NewWindow builds rolling return windows sized from config.
*/
func NewWindow() *Window {
	windowCap := config.System.MinCorrelationSamples

	if config.System.PriceHistory > 0 && config.System.PriceHistory < windowCap {
		windowCap = config.System.PriceHistory
	}

	return &Window{
		lastPrices: make(map[string]float64),
		prices:     make(map[string]correlation.PriceSampleRing),
		windowCap:  windowCap,
		minSamples: config.System.MinCorrelationSamples,
	}
}

/*
ObserveSymbol records one price tick at the current wall clock.
*/
func (window *Window) ObserveSymbol(symbol string, price float64) {
	window.ObserveSymbolAt(symbol, price, time.Now())
}

/*
ObserveSymbolAt records one price tick for grid-resampled correlation.
*/
func (window *Window) ObserveSymbolAt(symbol string, price float64, at time.Time) {
	if window == nil || symbol == "" || price <= 0 {
		return
	}

	if at.IsZero() {
		at = time.Now()
	}

	sampleWindow, ok := window.prices[symbol]

	if !ok {
		sampleWindow = correlation.NewPriceSampleRing(window.windowCap)
	}

	sampleWindow.Push(at, price)
	window.prices[symbol] = sampleWindow
	window.lastPrices[symbol] = price
}

/*
Mark returns the latest observed price for one symbol.
*/
func (window *Window) Mark(symbol string) float64 {
	if window == nil {
		return 0
	}

	return window.lastPrices[symbol]
}

/*
Marks returns the latest observed price for each symbol.
*/
func (window *Window) Marks() map[string]float64 {
	if window == nil {
		return nil
	}

	return window.lastPrices
}

/*
MarketRegime classifies recent price path efficiency for one symbol.
*/
func (window *Window) MarketRegime(symbol string) engine.MarketRegime {
	if window == nil || symbol == "" {
		return engine.RegimeUnknown
	}

	samples := window.prices[symbol].Ordered()

	if len(samples) < window.minSamples+1 {
		return engine.RegimeUnknown
	}

	return window.regimeFromReturns(correlation.LogReturnsFromPrices(window.samplePrices(samples)))
}

/*
SymbolCorrelation measures synchronized log-return correlation between two symbols.
*/
func (window *Window) SymbolCorrelation(left, right string) (float64, bool) {
	if window == nil {
		return 0, false
	}

	leftSamples := window.prices[left].Ordered()
	rightSamples := window.prices[right].Ordered()

	if len(leftSamples) < 2 || len(rightSamples) < 2 {
		return 0, false
	}

	leftReturns, rightReturns, ok := correlation.SynchronizedLogReturns(
		leftSamples, rightSamples, correlation.BarInterval(),
	)

	if !ok || len(leftReturns) < window.minSamples {
		return 0, false
	}

	return correlation.Pearson(leftReturns, rightReturns), true
}

/*
BuildMatrix fills one symmetric correlation matrix from symbol price windows.
*/
func (window *Window) BuildMatrix(symbols []string) (*Matrix, bool) {
	if window == nil || len(symbols) < 2 {
		return nil, false
	}

	matrix := &Matrix{rows: make([][]float64, len(symbols))}

	for row := range matrix.rows {
		matrix.rows[row] = make([]float64, len(symbols))
		matrix.rows[row][row] = 1
	}

	for row := 0; row < len(symbols); row++ {
		for col := row + 1; col < len(symbols); col++ {
			pairCorrelation, ok := window.SymbolCorrelation(symbols[row], symbols[col])

			if !ok {
				return nil, false
			}

			pairCorrelation = math.Max(pairCorrelation, 0)
			matrix.rows[row][col] = pairCorrelation
			matrix.rows[col][row] = pairCorrelation
		}
	}

	return matrix, true
}

func (window *Window) samplePrices(samples []correlation.PriceSample) []float64 {
	prices := make([]float64, 0, len(samples))

	for _, sample := range samples {
		if sample.Price <= 0 {
			continue
		}

		prices = append(prices, sample.Price)
	}

	return prices
}

func (window *Window) regimeFromReturns(returns []float64) engine.MarketRegime {
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
