package types

import (
	"fmt"
	"math"
)

/*
priceSignificantFigures is the quoting precision of a generated instrument.
Keeping the same significant resolution across price magnitudes lets the
simulator represent both sub-cent and high-notional symbols without imposing a
cent tick on either one.
*/
const priceSignificantFigures = 5

const (
	DefaultQuantityPrecision      = 8
	DefaultBaseSpreadFraction     = 0.0005
	DefaultTakerFeePercent        = 0.26
	DefaultMakerFeePercent        = 0.16
	DefaultBookDepthLevels        = 1
	DefaultBookDepthQuantityScale = 1.0
	DefaultOrderMinimum           = 0.0001
	DefaultCostMinimum            = 0.50
)

/*
Symbol declares the venue characteristics of one simulated instrument.
*/
type Symbol struct {
	Pair               string
	StartPrice         float64
	PriceIncrement     float64
	PricePrecision     int
	QuantityPrecision  int
	BaseSpreadFraction float64
	TakerFeePercent    float64
	MakerFeePercent    float64
	OrderMinimum       float64
	CostMinimum        float64
	BookDepthLevels    int
	DepthQuantityScale float64
	FactorLoading      float64
	FactorLagTicks     int
	Seed               int64
}

func validateSymbol(symbol *Symbol) error {
	if symbol == nil || symbol.Pair == "" {
		return fmt.Errorf("scenario: symbol and pair are required")
	}

	if symbol.StartPrice <= 0 || symbol.PriceIncrement <= 0 ||
		math.IsNaN(symbol.StartPrice) || math.IsInf(symbol.StartPrice, 0) ||
		math.IsNaN(symbol.PriceIncrement) || math.IsInf(symbol.PriceIncrement, 0) {
		return fmt.Errorf("scenario: %s prices must be positive and finite", symbol.Pair)
	}

	if symbol.PricePrecision < 0 || symbol.QuantityPrecision < 0 {
		return fmt.Errorf("scenario: %s precision must be non-negative", symbol.Pair)
	}

	if symbol.BaseSpreadFraction <= 0 ||
		math.IsNaN(symbol.BaseSpreadFraction) ||
		math.IsInf(symbol.BaseSpreadFraction, 0) {
		return fmt.Errorf("scenario: %s spread fraction must be positive and finite", symbol.Pair)
	}

	if symbol.TakerFeePercent < 0 || symbol.MakerFeePercent < 0 ||
		math.IsNaN(symbol.TakerFeePercent) || math.IsNaN(symbol.MakerFeePercent) ||
		math.IsInf(symbol.TakerFeePercent, 0) || math.IsInf(symbol.MakerFeePercent, 0) {
		return fmt.Errorf("scenario: %s fees must be finite and non-negative", symbol.Pair)
	}

	if symbol.OrderMinimum <= 0 || symbol.CostMinimum <= 0 ||
		math.IsNaN(symbol.OrderMinimum) || math.IsNaN(symbol.CostMinimum) ||
		math.IsInf(symbol.OrderMinimum, 0) || math.IsInf(symbol.CostMinimum, 0) {
		return fmt.Errorf("scenario: %s order minima must be positive and finite", symbol.Pair)
	}

	if symbol.BookDepthLevels < 1 || symbol.DepthQuantityScale <= 0 ||
		math.IsNaN(symbol.DepthQuantityScale) ||
		math.IsInf(symbol.DepthQuantityScale, 0) {
		return fmt.Errorf("scenario: %s depth levels and quantity scale must be positive", symbol.Pair)
	}

	if symbol.FactorLoading < -1 || symbol.FactorLoading > 1 ||
		math.IsNaN(symbol.FactorLoading) {
		return fmt.Errorf("scenario: %s factor loading must be between negative one and one", symbol.Pair)
	}

	if symbol.FactorLagTicks < 0 {
		return fmt.Errorf("scenario: %s factor lag must be non-negative", symbol.Pair)
	}

	return nil
}

/*
NewSymbol derives a tick size from the starting price and applies the fixture
venue's documented default precision, spread, and fee tier.
*/
func NewSymbol(
	pair string,
	startPrice float64,
	seed int64,
) *Symbol {
	if startPrice <= 0 || math.IsNaN(startPrice) || math.IsInf(startPrice, 0) {
		panic("tests: simulated symbol requires a positive finite start price")
	}

	priceIncrement := math.Pow10(
		int(math.Floor(math.Log10(startPrice))) - priceSignificantFigures + 1,
	)
	pricePrecision := max(0, int(-math.Floor(math.Log10(priceIncrement))))

	return &Symbol{
		Pair:               pair,
		StartPrice:         startPrice,
		PriceIncrement:     priceIncrement,
		PricePrecision:     pricePrecision,
		QuantityPrecision:  DefaultQuantityPrecision,
		BaseSpreadFraction: DefaultBaseSpreadFraction,
		TakerFeePercent:    DefaultTakerFeePercent,
		MakerFeePercent:    DefaultMakerFeePercent,
		OrderMinimum:       DefaultOrderMinimum,
		CostMinimum:        DefaultCostMinimum,
		BookDepthLevels:    DefaultBookDepthLevels,
		DepthQuantityScale: DefaultBookDepthQuantityScale,
		Seed:               seed,
	}
}
