package resonance

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

func (signal *Signal[T]) observeBooks(rows kraken.BookDataSlice) error {
	for _, row := range rows {
		if row.Symbol == "" {
			return errnie.Err(errnie.Validation, "resonance: book symbol required", nil)
		}

		element, err := sonic.Marshal(row)

		if err != nil {
			return errnie.Err(errnie.Validation, "resonance: marshal book row", err)
		}

		signal.book.ingest(row.Symbol, element, observedAt(row.Timestamp))
		signal.markMarketChanged(row.Symbol)
	}

	return nil
}

func (signal *Signal[T]) observeTrades(rows kraken.TradeDataSlice) error {
	for _, row := range rows {
		if row.Symbol == "" {
			return errnie.Err(errnie.Validation, "resonance: trade symbol required", nil)
		}

		element, err := sonic.Marshal(row)

		if err != nil {
			return errnie.Err(errnie.Validation, "resonance: marshal trade row", err)
		}

		signal.trade.ingest(row.Symbol, element, observedAt(row.Timestamp))
		signal.markMarketChanged(row.Symbol)
	}

	return nil
}

func (signal *Signal[T]) observeTickers(rows kraken.TickerDataSlice) error {
	for _, row := range rows {
		if row.Symbol == "" {
			return errnie.Err(errnie.Validation, "resonance: ticker symbol required", nil)
		}

		element, err := sonic.Marshal(row)

		if err != nil {
			return errnie.Err(errnie.Validation, "resonance: marshal ticker row", err)
		}

		signal.ticker.ingest(row.Symbol, element, observedAt(row.Timestamp))
		signal.markMarketChanged(row.Symbol)
	}

	return nil
}

func peekElementOK[T any](element []byte, path string) (T, bool) {
	var zero T
	var payload any

	if len(element) == 0 || path == "" {
		return zero, false
	}

	if err := sonic.Unmarshal(element, &payload); err != nil {
		return zero, false
	}

	value, ok := nestedValue(payload, strings.Split(path, "."))

	if !ok {
		return zero, false
	}

	return typedValue[T](value)
}

func nestedValue(payload any, path []string) (any, bool) {
	current := payload

	for _, segment := range path {
		if segment == "" {
			return nil, false
		}

		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]

			if !ok {
				return nil, false
			}

			current = next
		case []any:
			index, err := strconv.Atoi(segment)

			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}

			current = typed[index]
		default:
			return nil, false
		}
	}

	return current, true
}

func typedValue[T any](value any) (T, bool) {
	var zero T

	switch any(zero).(type) {
	case float64:
		number, ok := floatValue(value)
		return any(number).(T), ok
	case string:
		text, ok := value.(string)
		return any(text).(T), ok
	case time.Time:
		observed, ok := timeValue(value)
		return any(observed).(T), ok
	default:
		return zero, false
	}
}

func floatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		number, err := typed.Float64()

		return number, err == nil
	default:
		return 0, false
	}
}

func timeValue(value any) (time.Time, bool) {
	text, ok := value.(string)

	if !ok || text == "" {
		return time.Time{}, false
	}

	observed, err := time.Parse(time.RFC3339Nano, text)

	if err != nil {
		return time.Time{}, false
	}

	return observed.UTC(), true
}

func elementTime(element []byte, key string) (time.Time, bool) {
	return peekElementOK[time.Time](element, key)
}
