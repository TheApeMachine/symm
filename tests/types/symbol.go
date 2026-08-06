package types

import "math"

/*
priceSignificantFigures is the quoting precision of a generated instrument.
Keeping the same significant resolution across price magnitudes lets the
simulator represent both sub-cent and high-notional symbols without imposing a
cent tick on either one.
*/
const priceSignificantFigures = 5

type Symbol struct {
	Pair           string
	StartPrice     float64
	PriceIncrement float64
	PricePrecision int
	Seed           int64
}

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
		Pair:           pair,
		StartPrice:     startPrice,
		PriceIncrement: priceIncrement,
		PricePrecision: pricePrecision,
		Seed:           seed,
	}
}
