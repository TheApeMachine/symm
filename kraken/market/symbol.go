package market

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Symbol is one scoped observation row for cross-section analytics.
*/
type Symbol struct {
	Name     string
	Price    float64
	Value    float64
	Volume   float64
	Pressure float64
	Updated  time.Time
}

/*
Validate rejects rows that cannot participate in cross-section scoring.
*/
func (symbol *Symbol) Validate() error {
	if symbol == nil {
		return errnie.Error(fmt.Errorf("symbol: nil row"))
	}

	if symbol.Name == "" {
		return errnie.Error(fmt.Errorf("symbol: empty name"))
	}

	if symbol.Price <= 0 {
		return errnie.Error(fmt.Errorf("symbol: non-positive price"))
	}

	if symbol.Volume <= 0 {
		return errnie.Error(fmt.Errorf("symbol: non-positive volume"))
	}

	return nil
}

/*
NewSymbolRow builds a validated symbol row from one observation.
*/
func NewSymbolRow(
	name string,
	price, value, volume, pressure float64,
	updated time.Time,
) (*Symbol, error) {
	row := &Symbol{
		Name:     name,
		Price:    price,
		Value:    value,
		Volume:   volume,
		Pressure: pressure,
		Updated:  updated,
	}

	if err := row.Validate(); err != nil {
		return nil, err
	}

	return row, nil
}

/*
SymbolRowFromPrices derives a row from a trade-price window.
*/
func SymbolRowFromPrices(
	name string,
	prices []float64,
	quoteVolume, pressure float64,
	updated time.Time,
) (*Symbol, error) {
	if len(prices) < 2 {
		return nil, errnie.Error(fmt.Errorf("symbol: need at least two prices"))
	}

	first := prices[0]
	last := prices[len(prices)-1]

	if first <= 0 || last <= 0 {
		return nil, errnie.Error(fmt.Errorf("symbol: non-positive prices"))
	}

	change := (last - first) / first

	return NewSymbolRow(name, last, change, quoteVolume, pressure, updated)
}

/*
CompleteSymbol maps a ticker update into a cross-section row.
*/
func (ticker *TickerUpdate) CompleteSymbol(pressure float64, at time.Time) (*Symbol, error) {
	if ticker == nil {
		return nil, errnie.Error(fmt.Errorf("symbol: nil ticker"))
	}

	price := ticker.Last

	if price <= 0 {
		price = (ticker.Ask + ticker.Bid) / 2
	}

	change := ticker.ChangePct

	if change == 0 && ticker.Change != 0 && price > 0 {
		change = ticker.Change / price
	}

	volume := ticker.Volume

	if volume <= 0 {
		volume = ticker.VWAP * ticker.Volume
	}

	if volume <= 0 {
		volume = price
	}

	return NewSymbolRow(ticker.Symbol, price, change, volume, pressure, at)
}
