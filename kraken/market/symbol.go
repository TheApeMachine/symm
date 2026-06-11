package market

import (
	"errors"
	"math"
	"time"

	"github.com/theapemachine/errnie"
)

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

	if src.Price != symbol.Price {
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
