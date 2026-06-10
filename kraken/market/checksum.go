package market

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
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

func (level BookLevel) qtyChecksumToken() string {
	wire := level.qtyWire

	if wire == "" {
		wire = floatWire(level.Qty)
	}

	return wireChecksumToken(wire)
}

/*
checksumBook computes Kraken v2 CRC32 over the top N ask then bid levels.
Wire strings from the feed must be preserved; float round-trips break the hash.
*/
func checksumBook(book *BookUpdate, levels int) uint32 {
	if book == nil {
		return 0
	}

	builder := strings.Builder{}

	askCount := len(book.Asks)

	if askCount > levels {
		askCount = levels
	}

	for index := range askCount {
		builder.WriteString(book.Asks[index].priceChecksumToken())
		builder.WriteString(book.Asks[index].qtyChecksumToken())
	}

	bidCount := len(book.Bids)

	if bidCount > levels {
		bidCount = levels
	}

	for index := range bidCount {
		builder.WriteString(book.Bids[index].priceChecksumToken())
		builder.WriteString(book.Bids[index].qtyChecksumToken())
	}

	return crc32.ChecksumIEEE([]byte(builder.String()))
}
