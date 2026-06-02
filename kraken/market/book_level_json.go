package market

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
)

type bookLevelWire struct {
	Price any `json:"price"`
	Qty   any `json:"qty"`
}

/*
UnmarshalJSON accepts Kraken book levels where price and qty arrive as strings or numbers.
*/
func (level *BookLevel) UnmarshalJSON(data []byte) error {
	var wire bookLevelWire

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return err
	}

	price, err := parseBookField(wire.Price, "price")

	if err != nil {
		return err
	}

	qty, err := parseBookField(wire.Qty, "qty")

	if err != nil {
		return err
	}

	level.Price = price
	level.Qty = qty
	level.PriceRaw = formatBookRaw(wire.Price, price)
	level.QtyRaw = formatBookRaw(wire.Qty, qty)

	return nil
}

func parseBookField(raw any, name string) (float64, error) {
	switch value := raw.(type) {
	case nil:
		return 0, fmt.Errorf("market: book level missing %s", name)
	case float64:
		return value, nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)

		if err != nil {
			return 0, fmt.Errorf("market: book level %s %q: %w", name, value, err)
		}

		return parsed, nil
	default:
		return 0, fmt.Errorf("market: book level %s has unsupported type %T", name, raw)
	}
}

func formatBookRaw(raw any, parsed float64) string {
	if text, ok := raw.(string); ok && text != "" {
		return text
	}

	return strconv.FormatFloat(parsed, 'f', -1, 64)
}
