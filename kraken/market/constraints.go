package market

import (
	"fmt"
	"math"
)

/*
InstrumentConstraints are the exchange sizing rules for one symbol.
*/
type InstrumentConstraints struct {
	Symbol         string
	QtyIncrement   float64
	QtyMin         float64
	CostMin        float64
	PriceIncrement float64
}

func ConstraintsFromPair(pair InstrumentPair) InstrumentConstraints {
	return InstrumentConstraints{
		Symbol:         pair.Symbol,
		QtyIncrement:   pair.QtyIncrement,
		QtyMin:         pair.QtyMin,
		CostMin:        pair.CostMin,
		PriceIncrement: pair.PriceIncrement,
	}
}

/*
QuantizeDown floors value to the nearest increment without exceeding it.
*/
func QuantizeDown(value float64, increment float64) (float64, error) {
	if increment <= 0 {
		return 0, fmt.Errorf("market: increment must be positive")
	}

	if value <= 0 {
		return 0, fmt.Errorf("market: value must be positive")
	}

	steps := math.Floor(value/increment + 1e-12)

	if steps <= 0 {
		return 0, fmt.Errorf("market: quantized value rounds to zero")
	}

	return steps * increment, nil
}

/*
QuantizePrice rounds a price to the nearest valid tick.
*/
func QuantizePrice(price float64, increment float64) (float64, error) {
	if increment <= 0 {
		return 0, fmt.Errorf("market: price increment must be positive")
	}

	if price <= 0 {
		return 0, fmt.Errorf("market: price must be positive")
	}

	steps := math.Round(price/increment + 1e-12)

	if steps <= 0 {
		return 0, fmt.Errorf("market: quantized price rounds to zero")
	}

	return steps * increment, nil
}
