package market

import (
	"fmt"
	"time"

	"github.com/theapemachine/datura"
)

/*
Symbol is one cross-section observation row for peer-relative analytics.
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
Validate checks that the row has the minimum fields required for Observe.
*/
func (row *Symbol) Validate() error {
	if row == nil {
		return fmt.Errorf("cross-section: nil row")
	}

	if row.Name == "" {
		return fmt.Errorf("cross-section: empty symbol name")
	}

	if row.Price <= 0 {
		return fmt.Errorf("cross-section: price must be positive")
	}

	return nil
}

/*
NewSymbolRow builds one validated cross-section row.
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

	return row, row.Validate()
}

/*
SymbolFromTicker parses one Kraken ticker row into a cross-section row.
*/
func SymbolFromTicker(datapoint *datura.Artifact, rowIndex int) (*Symbol, error) {
	if datapoint == nil {
		return nil, fmt.Errorf("cross-section: nil datapoint")
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "" && channel != "ticker" {
		return nil, fmt.Errorf("cross-section: expected ticker channel")
	}

	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
	last := datura.Peek[float64](datapoint, "data", rowIndex, "last")
	volume := datura.Peek[float64](datapoint, "data", rowIndex, "volume")
	changePct := datura.Peek[float64](datapoint, "data", rowIndex, "change_pct")
	bidQty := datura.Peek[float64](datapoint, "data", rowIndex, "bid_qty")
	askQty := datura.Peek[float64](datapoint, "data", rowIndex, "ask_qty")
	updated := time.Unix(0, datapoint.Timestamp())

	if updated.IsZero() {
		updated = time.Now().UTC()
	}

	pressure := 0.0
	bookDepth := bidQty + askQty

	if bookDepth > 0 {
		pressure = (bidQty - askQty) / bookDepth
	}

	return NewSymbolRow(symbol, last, changePct/100, volume, pressure, updated)
}
