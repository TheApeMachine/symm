package market

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Positions keeps the live ledger needed by the UI without reading the tree as a
message bus.
*/
type Positions struct {
	quote    string
	balances map[string]float64
	entries  map[string]float64
	marks    map[string]float64
	stops    map[string]map[string]any
	updated  int64
}

type openPosition struct {
	asset    string
	symbol   string
	quantity float64
}

func NewPositions(quoteCurrency string) *Positions {
	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))
	if quote == "" {
		quote = "USD"
	}

	return &Positions{
		quote:    quote,
		balances: map[string]float64{},
		entries:  map[string]float64{},
		marks:    map[string]float64{},
		stops:    map[string]map[string]any{},
	}
}

func (positions *Positions) Quote() string {
	return positions.quote
}

func (positions *Positions) Observe(frame map[string]any) error {
	role := frameText(frame, "role")
	if role == "" {
		role = frameText(frame, "channel")
	}

	switch role {
	case "balances":
		return positions.observeBalances(frame)
	case "executions", "fill":
		return positions.observeExecutions(frame)
	case "ticker", "quote":
		return positions.observeTicker(frame)
	case "stoploss":
		return positions.observeStop(frame)
	default:
		return nil
	}
}

func (positions *Positions) Readings() ([]map[string]any, error) {
	if positions == nil || positions.balances == nil {
		return nil, nil
	}

	opened := positions.open()
	if len(opened) == 0 {
		return []map[string]any{}, nil
	}

	readings := make([]map[string]any, 0, len(opened))
	for _, position := range opened {
		reading, err := positions.reading(position)
		if err != nil {
			return nil, err
		}

		readings = append(readings, reading)
	}

	sort.Slice(readings, func(first, second int) bool {
		return strings.Compare(
			fmt.Sprint(readings[first]["symbol"]),
			fmt.Sprint(readings[second]["symbol"]),
		) < 0
	})

	return readings, nil
}

func (positions *Positions) observeBalances(frame map[string]any) error {
	rows := frameRows(frame, "data")
	next := map[string]float64{}

	for _, row := range rows {
		asset := strings.ToUpper(frameText(row, "asset"))
		if asset == "" {
			return errnie.Err(errnie.Validation, "positions: balance asset required", nil)
		}

		next[asset] = frameFloat(row, "balance")
	}

	if len(next) == 0 {
		return errnie.Err(errnie.Validation, "positions: balances data required", nil)
	}

	positions.balances = next
	positions.updated = frameTime(frame)
	return nil
}

func (positions *Positions) observeExecutions(frame map[string]any) error {
	for _, row := range executionRows(frame) {
		side := strings.ToLower(frameText(row, "side"))
		if side != "buy" && side != "enter" {
			continue
		}

		symbol := strings.ToUpper(frameText(row, "symbol"))
		if symbol == "" {
			continue
		}

		price := frameFloat(row, "avg_price")
		if price <= 0 {
			price = frameFloat(row, "last_price")
		}
		if price <= 0 {
			price = frameFloat(row, "price")
		}
		if price <= 0 {
			return errnie.Err(
				errnie.Validation,
				"positions: non-positive execution price for "+symbol,
				nil,
			)
		}

		positions.entries[symbol] = price
	}

	return nil
}

func (positions *Positions) observeTicker(frame map[string]any) error {
	for _, row := range frameRows(frame, "data") {
		symbol := strings.ToUpper(frameText(row, "symbol"))
		if symbol == "" {
			continue
		}

		price := frameFloat(row, "last")
		if price <= 0 {
			price = frameFloat(row, "price")
		}
		if price <= 0 {
			continue
		}

		positions.marks[symbol] = price
	}

	return nil
}

func (positions *Positions) observeStop(frame map[string]any) error {
	symbol := strings.ToUpper(frameText(frame, "symbol"))
	if symbol == "" {
		symbol = strings.ToUpper(frameText(frame, "scope"))
	}
	if symbol == "" {
		return errnie.Err(errnie.Validation, "positions: stop symbol required", nil)
	}

	positions.stops[symbol] = frame
	return nil
}

func (positions *Positions) open() []openPosition {
	out := make([]openPosition, 0)

	for asset, quantity := range positions.balances {
		if asset == positions.quote || quantity <= 0 {
			continue
		}

		out = append(out, openPosition{
			asset:    asset,
			symbol:   asset + "/" + positions.quote,
			quantity: quantity,
		})
	}

	return out
}

func (positions *Positions) reading(position openPosition) (map[string]any, error) {
	reading := map[string]any{
		"symbol":    position.symbol,
		"asset":     position.asset,
		"quote":     positions.quote,
		"quantity":  position.quantity,
		"updatedAt": positions.updated,
	}

	mark := positions.marks[strings.ToUpper(position.symbol)]
	entry := positions.entries[strings.ToUpper(position.symbol)]

	if mark > 0 {
		reading["mark"] = mark
		reading["value"] = mark * position.quantity
	} else {
		reading["status"] = "missing_mark"
	}

	if entry > 0 {
		reading["entry"] = entry
	} else if reading["status"] == nil {
		reading["status"] = "missing_entry"
	} else {
		reading["status"] = "missing_mark_entry"
	}

	if mark > 0 && entry > 0 {
		reading["unrealizedPnl"] = (mark - entry) * position.quantity
		reading["changePct"] = (mark - entry) / entry * 100
		reading["status"] = "marked"
	}

	if stop := positions.stops[strings.ToUpper(position.symbol)]; stop != nil {
		reading["stop"] = frameFloat(stop, "stop")
		reading["peak"] = frameFloat(stop, "peak")
		reading["offset"] = frameFloat(stop, "offset")
		reading["stopSide"] = frameText(stop, "side")
	}

	return reading, nil
}

func executionRows(frame map[string]any) []map[string]any {
	rows := frameRows(frame, "data")
	if len(rows) > 0 {
		return rows
	}

	values, _ := frame["executions"].(map[string]any)
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		row, ok := value.(map[string]any)
		if ok {
			out = append(out, row)
		}
	}

	return out
}

func frameRows(frame map[string]any, key string) []map[string]any {
	values, _ := frame[key].([]map[string]any)
	if len(values) > 0 {
		return values
	}

	items, _ := frame[key].([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if ok {
			rows = append(rows, row)
		}
	}

	return rows
}

func frameText(frame map[string]any, key string) string {
	if frame[key] == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(frame[key]))
}

func frameFloat(frame map[string]any, key string) float64 {
	switch value := frame[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return parsed
		}
	}

	return 0
}

func frameTime(frame map[string]any) int64 {
	value := frameText(frame, "timestamp")
	if value == "" {
		return time.Now().UTC().UnixNano()
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Now().UTC().UnixNano()
	}

	return parsed.UnixNano()
}
