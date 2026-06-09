package market

import (
	"math"
	"time"
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

func (symbol *Symbol) Update(src *Symbol, returnCap int) {
	if src.Price > 0 && symbol.Price > 0 && src.Price != symbol.Price {
		symbol.Returns = append(symbol.Returns, math.Log(src.Price/symbol.Price))
		if len(symbol.Returns) > returnCap {
			symbol.Returns = symbol.Returns[len(symbol.Returns)-returnCap:]
		}
	}

	if src.Price > 0 {
		symbol.Price = src.Price
	}

	if src.Value != 0 {
		symbol.Value = src.Value
	}

	if src.Volume > 0 {
		symbol.Volume = src.Volume
	}

	if src.Pressure != 0 {
		symbol.Pressure = src.Pressure
	}

	if !src.Updated.IsZero() {
		symbol.Updated = src.Updated
	}
}
