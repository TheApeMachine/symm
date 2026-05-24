package order

import (
	"encoding/json"
	"fmt"
	"strings"
)

/*
Fill is one trade execution extracted from the executions channel.
*/
type Fill struct {
	OrderID string
	Symbol  string
	Side    string
	Qty     float64
	Price   float64
}

/*
ExecutionEvent is one Kraken v2 executions row.
*/
type ExecutionEvent struct {
	OrderID   string
	Symbol    string
	Side      string
	OrderType string
	ExecType  string
	OrdRefID  string
	LastQty   float64
	LastPrice float64
}

/*
ParseExecutionFills extracts trade fills from a Kraken v2 executions frame.
*/
func ParseExecutionFills(payload []byte) ([]Fill, error) {
	events, err := ParseExecutionEvents(payload)

	if err != nil {
		return nil, err
	}

	fills := make([]Fill, 0, len(events))

	for _, event := range events {
		if event.ExecType != "trade" {
			continue
		}

		if event.LastQty <= 0 || event.LastPrice <= 0 {
			continue
		}

		fills = append(fills, Fill{
			OrderID: event.OrderID,
			Symbol:  event.Symbol,
			Side:    event.Side,
			Qty:     event.LastQty,
			Price:   event.LastPrice,
		})
	}

	return fills, nil
}

/*
ParseExecutionEvents extracts all execution rows from one executions frame.
*/
func ParseExecutionEvents(payload []byte) ([]ExecutionEvent, error) {
	var frame struct {
		Channel string `json:"channel"`
		Data    []struct {
			OrderID   string  `json:"order_id"`
			Symbol    string  `json:"symbol"`
			Side      string  `json:"side"`
			OrderType string  `json:"order_type"`
			ExecType  string  `json:"exec_type"`
			OrdRefID  string  `json:"ord_ref_id"`
			LastQty   float64 `json:"last_qty"`
			LastPrice float64 `json:"last_price"`
		} `json:"data"`
	}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal executions frame: %w", err)
	}

	if frame.Channel != "executions" {
		return nil, fmt.Errorf("not an executions frame: %q", frame.Channel)
	}

	events := make([]ExecutionEvent, 0, len(frame.Data))

	for _, row := range frame.Data {
		events = append(events, ExecutionEvent{
			OrderID:   row.OrderID,
			Symbol:    row.Symbol,
			Side:      row.Side,
			OrderType: row.OrderType,
			ExecType:  row.ExecType,
			OrdRefID:  row.OrdRefID,
			LastQty:   row.LastQty,
			LastPrice: row.LastPrice,
		})
	}

	return events, nil
}

/*
FindOTOStopOrderID returns the resting stop order spawned by one primary fill.
*/
func FindOTOStopOrderID(events []ExecutionEvent, parentOrderID, symbol string) string {
	for _, event := range events {
		if event.ExecType != "new" {
			continue
		}

		if event.OrderID == "" || event.OrderID == parentOrderID {
			continue
		}

		if event.OrdRefID != parentOrderID {
			continue
		}

		if symbol != "" && event.Symbol != symbol {
			continue
		}

		if event.Side != "sell" {
			continue
		}

		if !isStopOrderType(event.OrderType) {
			continue
		}

		return event.OrderID
	}

	return ""
}

func isStopOrderType(orderType string) bool {
	normalized := strings.ToLower(orderType)

	return strings.Contains(normalized, "stop")
}
