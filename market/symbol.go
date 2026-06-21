package market

import (
	"fmt"
	"time"
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
