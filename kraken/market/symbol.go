package market

import (
	"errors"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/numeric"
)

var ErrFlatPriceWindow = errors.New("kraken: flat price window")

type Symbol struct {
	Name     string
	Quote    string
	Price    float64
	Value    float64
	Updated  time.Time
	Returns  []float64
	Volume   float64
	Pressure float64
}

func (symbol *Symbol) Update(src *Symbol, returnCap int) error {
	if src == nil {
		return errnie.Error(errors.New("kraken: symbol update is nil"))
	}

	if err := src.Validate(); err != nil {
		return errnie.Error(err)
	}

	if symbol.Price > 0 && src.Price > 0 && src.Price != symbol.Price {
		symbol.Returns = append(symbol.Returns, math.Log(src.Price/symbol.Price))

		if len(symbol.Returns) > returnCap {
			symbol.Returns = symbol.Returns[len(symbol.Returns)-returnCap:]
		}
	}

	symbol.Price = src.Price
	symbol.Value = src.Value
	symbol.Volume = src.Volume
	symbol.Pressure = src.Pressure
	symbol.Updated = src.Updated

	return nil
}

/*
NewSymbolRow builds and validates a complete cross-section row.
*/
func NewSymbolRow(
	name string,
	price, value, volume, pressure float64,
	at time.Time,
) (*Symbol, error) {
	row := &Symbol{
		Name:     name,
		Price:    price,
		Value:    value,
		Volume:   volume,
		Pressure: pressure,
		Updated:  at,
	}

	if err := row.Validate(); err != nil {
		return nil, errnie.Error(err)
	}

	return row, nil
}

/*
SymbolRowFromPrices builds and validates a row from a price window.
*/
func SymbolRowFromPrices(
	name string,
	prices []float64,
	volume, pressure float64,
	at time.Time,
) (*Symbol, error) {
	if len(prices) == 0 {
		return nil, errnie.Error(errors.New("kraken: prices are required"))
	}

	price := prices[len(prices)-1]
	_, change := numeric.AnchorChange(prices[0], price)

	if change == 0 {
		change = microstructureValue(prices)

		if change <= 0 {
			return nil, ErrFlatPriceWindow
		}
	}

	return NewSymbolRow(name, price, change, volume, pressure, at)
}

func microstructureValue(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	moves := make([]float64, 0, len(prices)-1)

	for index := 1; index < len(prices); index++ {
		moves = append(moves, prices[index]-prices[index-1])
	}

	spread := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(moves...)...))
	price := prices[len(prices)-1]

	if price <= 0 || spread <= 0 {
		return 0
	}

	return spread / price
}
