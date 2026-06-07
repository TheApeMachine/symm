package market

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
)

type bookLevelWire struct {
	Price json.RawMessage `json:"price"`
	Qty   json.RawMessage `json:"qty"`
}

/*
UnmarshalJSON accepts Kraken book levels where price and qty arrive as strings
or numbers, preserving the literal wire token as the raw representation.

The v2 book checksum is defined over the decimal strings AS TRANSMITTED, so a
numeric qty of 0.10000000 must stay "0.10000000". The previous decoder
round-tripped numbers through float64 and strconv.FormatFloat('f', -1), which
collapses trailing zeros to "0.1" — making ComputedChecksum disagree with the
exchange on virtually every live frame (the wire sends numbers) while the
string-typed doc sample in the tests kept passing. That was the universal
"book checksum diverged" within seconds of connect on 2026-06-07.
*/
func (level *BookLevel) UnmarshalJSON(data []byte) error {
	var wire bookLevelWire

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return err
	}

	priceRaw, price, err := parseBookToken(wire.Price, "price")

	if err != nil {
		return err
	}

	qtyRaw, qty, err := parseBookToken(wire.Qty, "qty")

	if err != nil {
		return err
	}

	level.Price = price
	level.Qty = qty
	level.PriceRaw = priceRaw
	level.QtyRaw = qtyRaw

	return nil
}

// parseBookToken returns the unquoted literal wire token and its parsed value.
func parseBookToken(token json.RawMessage, name string) (string, float64, error) {
	text := strings.TrimSpace(string(token))

	if text == "" || text == "null" {
		return "", 0, fmt.Errorf("market: book level missing %s", name)
	}

	if len(text) >= 2 && text[0] == '"' {
		unquoted, err := strconv.Unquote(text)

		if err != nil {
			return "", 0, fmt.Errorf("market: book level %s %s: %w", name, text, err)
		}

		text = unquoted
	}

	parsed, err := strconv.ParseFloat(text, 64)

	if err != nil {
		return "", 0, fmt.Errorf("market: book level %s %q: %w", name, text, err)
	}

	return text, parsed, nil
}
