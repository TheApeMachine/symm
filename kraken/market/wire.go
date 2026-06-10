package market

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
)

func parseWireAmount(raw json.RawMessage) (wire string, value float64, err error) {
	trimmed := bytes.TrimSpace(raw)

	if len(trimmed) == 0 {
		return "", 0, fmt.Errorf("market: empty wire amount")
	}

	if trimmed[0] == '"' {
		if err := sonic.Unmarshal(trimmed, &wire); err != nil {
			return "", 0, err
		}
	} else {
		wire = string(trimmed)
	}

	value, err = strconv.ParseFloat(wire, 64)

	if err != nil {
		return "", 0, fmt.Errorf("market: parse wire amount %q: %w", wire, err)
	}

	return wire, value, nil
}

func floatWire(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func wireChecksumToken(wire string) string {
	stripped := strings.ReplaceAll(wire, ".", "")
	stripped = strings.TrimLeft(stripped, "0")

	if stripped == "" {
		return "0"
	}

	return stripped
}

func (level BookLevel) priceChecksumToken() string {
	wire := level.priceWire

	if wire == "" {
		wire = floatWire(level.Price)
	}

	return wireChecksumToken(wire)
}
