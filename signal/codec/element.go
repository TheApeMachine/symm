package codec

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
)

/*
PeekElementOK reads one typed field from a JSON feed element.
*/
func PeekElementOK[T any](element []byte, path string) (T, bool) {
	var zero T

	if len(element) == 0 || path == "" {
		return zero, false
	}

	root, parseErr := sonic.Get(element)

	if parseErr != nil {
		return zero, false
	}

	node, ok := elementNode(&root, path)

	if !ok {
		return zero, false
	}

	value, valueErr := nodeInterface(node)

	if valueErr != nil {
		return zero, false
	}

	typed, ok := coerceValue[T](value)

	return typed, ok
}

/*
ElementTime parses a timestamp field from a JSON feed element.
*/
func ElementTime(element []byte, key string) (time.Time, bool) {
	text, ok := PeekElementOK[string](element, key)

	if !ok || text == "" {
		return time.Time{}, false
	}

	parsed, parseErr := time.Parse(time.RFC3339Nano, text)

	if parseErr == nil {
		return parsed, true
	}

	parsed, parseErr = time.Parse(time.RFC3339, text)

	if parseErr != nil {
		return time.Time{}, false
	}

	return parsed, true
}

/*
EachBookLevelElement visits every level in a bids or asks array.
*/
func EachBookLevelElement(
	element []byte,
	key string,
	visit func(price float64, qty float64),
) {
	if len(element) == 0 || key == "" || visit == nil {
		return
	}

	for index := 0; ; index++ {
		priceNode, priceErr := sonic.Get(element, key, index, "price")
		qtyNode, qtyErr := sonic.Get(element, key, index, "qty")

		if priceErr != nil || qtyErr != nil || !priceNode.Exists() || !qtyNode.Exists() {
			return
		}

		price, priceOK := nodeFloat(&priceNode)
		qty, qtyOK := nodeFloat(&qtyNode)

		if !priceOK || !qtyOK {
			continue
		}

		visit(price, qty)
	}
}

/*
UnmarshalElement decodes a JSON feed element into dest.
*/
func UnmarshalElement(element []byte, dest any) error {
	if len(element) == 0 {
		return fmt.Errorf("codec: empty element")
	}

	if dest == nil {
		return fmt.Errorf("codec: nil destination")
	}

	return sonic.Unmarshal(element, dest)
}

/*
TouchSpread returns the price range across a trade or mid series.
*/
func TouchSpread(prices []float64) (float64, bool) {
	if len(prices) < 2 {
		return 0, false
	}

	minPrice := prices[0]
	maxPrice := prices[0]

	for _, price := range prices[1:] {
		if !finiteFloat(price) || price <= 0 {
			continue
		}

		if price < minPrice {
			minPrice = price
		}

		if price > maxPrice {
			maxPrice = price
		}
	}

	spread := maxPrice - minPrice

	if spread <= 0 {
		return 0, false
	}

	return spread, true
}

func elementNode(root *ast.Node, path string) (*ast.Node, bool) {
	if root == nil || !root.Exists() {
		return nil, false
	}

	node := root
	segments := strings.Split(path, ".")

	for _, segment := range segments {
		if segment == "" {
			return nil, false
		}

		index, indexErr := strconv.Atoi(segment)

		if indexErr == nil {
			node = node.Index(index)
		}

		if indexErr != nil {
			node = node.Get(segment)
		}

		if node == nil || !node.Exists() {
			return nil, false
		}
	}

	return node, true
}

func nodeFloat(node *ast.Node) (float64, bool) {
	if node == nil || !node.Exists() {
		return 0, false
	}

	value, valueErr := node.Float64()

	if valueErr == nil {
		return value, true
	}

	text, textErr := node.String()

	if textErr != nil {
		return 0, false
	}

	parsed, parseErr := strconv.ParseFloat(text, 64)

	if parseErr != nil {
		return 0, false
	}

	return parsed, true
}

func nodeInterface(node *ast.Node) (any, error) {
	if node == nil || !node.Exists() {
		return nil, fmt.Errorf("codec: missing node")
	}

	raw, rawErr := node.Raw()

	if rawErr != nil {
		return nil, rawErr
	}

	if raw == "null" {
		return nil, fmt.Errorf("codec: null node")
	}

	if raw == "true" {
		return true, nil
	}

	if raw == "false" {
		return false, nil
	}

	if strings.HasPrefix(raw, `"`) {
		text, textErr := node.String()

		return text, textErr
	}

	if strings.ContainsAny(raw, ".eE") {
		return node.Float64()
	}

	if integer, integerErr := node.Int64(); integerErr == nil {
		return integer, nil
	}

	return node.Float64()
}

func coerceValue[T any](value any) (T, bool) {
	var zero T

	switch typed := any(zero).(type) {
	case string:
		text, ok := value.(string)

		return any(text).(T), ok
	case float64:
		switch cast := value.(type) {
		case float64:
			return any(cast).(T), true
		case string:
			parsed, parseErr := strconv.ParseFloat(cast, 64)

			if parseErr != nil {
				return zero, false
			}

			return any(parsed).(T), true
		case int64:
			return any(float64(cast)).(T), true
		case int:
			return any(float64(cast)).(T), true
		}
	case int:
		switch cast := value.(type) {
		case int:
			return any(cast).(T), true
		case int64:
			return any(int(cast)).(T), true
		case float64:
			return any(int(cast)).(T), true
		}
	case int64:
		switch cast := value.(type) {
		case int64:
			return any(cast).(T), true
		case int:
			return any(int64(cast)).(T), true
		case float64:
			return any(int64(cast)).(T), true
		}
	case bool:
		boolean, ok := value.(bool)

		return any(boolean).(T), ok
	default:
		_ = typed
	}

	return zero, false
}

func finiteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}