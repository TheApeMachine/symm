package signal

import (
	"strconv"
	"time"

	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/datura"
)

func payloadString(node ast.Node, key string) (string, bool) {
	field := node.Get(key)

	if field == nil {
		return "", false
	}

	value, err := field.String()

	return value, err == nil
}

func payloadFloat(node ast.Node, key string) (float64, bool) {
	field := node.Get(key)

	if field == nil {
		return 0, false
	}

	value, err := field.Float64()

	if err == nil {
		return value, true
	}

	wire, wireErr := field.String()

	if wireErr != nil {
		return 0, false
	}

	parsed, parseErr := strconv.ParseFloat(wire, 64)

	return parsed, parseErr == nil
}

func payloadTime(node ast.Node, key string) (time.Time, bool) {
	wire, wireOK := payloadString(node, key)

	if !wireOK || wire == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339Nano, wire)

	if err != nil {
		parsed, err = time.Parse(time.RFC3339, wire)
	}

	return parsed, err == nil
}

func payloadBookLevels(node ast.Node, key string) []BookLevelRecord {
	arrayNode := node.Get(key)

	if arrayNode == nil {
		return nil
	}

	elements, err := arrayNode.ArrayUseNode()

	if err != nil {
		return nil
	}

	levels := make([]BookLevelRecord, 0, len(elements))

	for _, element := range elements {
		price, priceOK := payloadFloat(element, "price")
		qty, qtyOK := payloadFloat(element, "qty")

		if !priceOK || !qtyOK || price <= 0 {
			continue
		}

		levels = append(levels, BookLevelRecord{Price: price, Qty: qty})
	}

	return levels
}

/*
PayloadSymbols returns unique symbol fields from a market feed artifact payload.
*/
func PayloadSymbols(artifact *datura.Artifact) []string {
	if artifact == nil {
		return nil
	}

	seen := make(map[string]struct{})
	symbols := make([]string, 0, 4)

	datura.PayloadEach(artifact, func(index int, element ast.Node) bool {
		symbol, symbolOK := payloadString(element, "symbol")

		if !symbolOK || symbol == "" {
			return true
		}

		if _, exists := seen[symbol]; exists {
			return true
		}

		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)

		return true
	})

	return symbols
}
