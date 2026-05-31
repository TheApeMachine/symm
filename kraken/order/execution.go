package order

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

/*
Fill is one trade execution extracted from the executions channel. ExecKey is
the per-execution unique identifier used by Wallet.ApplyFill to dedupe
reconnect-replays — when ExecID is empty the parser falls back to
OrderID:TradeID:Timestamp.
*/
type Fill struct {
	OrderID  string
	ClOrdID  string
	Symbol   string
	Side     string
	Qty      float64
	Price    float64
	CumQty   float64
	OrderQty float64
	ExecKey  string
	Fee      float64
	FeeCcy   string
}

/*
ExecutionEvent is one Kraken v2 executions row. TradeID is an integer per
the v2 schema (docs.kraken.com/api/docs/websocket-v2/executions). ExecID is
a string UUID used as the unique key for fill dedupe.
*/
type ExecutionEvent struct {
	OrderID   string
	ClOrdID   string
	Symbol    string
	Side      string
	OrderType string
	ExecType  string
	OrdRefID  string
	ExecID    string
	TradeID   int64
	Timestamp string
	LastQty   float64
	LastPrice float64
	CumQty    float64
	OrderQty  float64
	Fee       float64
	FeeUSD    float64
	FeeCcy    string
}

/*
OrderFillTerminal reports whether an execution row completes the parent order.
When order_qty is absent (paper simulator), one trade fill is treated as terminal.
*/
func OrderFillTerminal(fill Fill) bool {
	if fill.OrderQty <= 0 {
		return true
	}

	return fill.CumQty >= fill.OrderQty
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

		execKey, err := execKeyFor(event)

		if err != nil {
			continue
		}

		fee := event.Fee

		if fee <= 0 && event.FeeUSD > 0 {
			fee = event.FeeUSD
		}

		fills = append(fills, Fill{
			OrderID:  event.OrderID,
			ClOrdID:  event.ClOrdID,
			Symbol:   event.Symbol,
			Side:     event.Side,
			Qty:      event.LastQty,
			Price:    event.LastPrice,
			CumQty:   event.CumQty,
			OrderQty: event.OrderQty,
			ExecKey:  execKey,
			Fee:      fee,
			FeeCcy:   event.FeeCcy,
		})
	}

	return fills, nil
}

func execKeyFor(event ExecutionEvent) (string, error) {
	if event.ExecID != "" {
		return event.ExecID, nil
	}

	if event.TradeID > 0 {
		return event.OrderID + ":" + strconv.FormatInt(event.TradeID, 10), nil
	}

	if event.Timestamp != "" {
		return event.OrderID + ":" + event.Timestamp, nil
	}

	return "", fmt.Errorf("execution missing dedupe key")
}

/*
ParseExecutionEvents extracts all execution rows from one executions frame.
*/
func ParseExecutionEvents(payload []byte) ([]ExecutionEvent, error) {
	var frame struct {
		Channel string `json:"channel"`
		Data    []struct {
			OrderID   string  `json:"order_id"`
			ClOrdID   string  `json:"cl_ord_id"`
			Symbol    string  `json:"symbol"`
			Side      string  `json:"side"`
			OrderType string  `json:"order_type"`
			ExecType  string  `json:"exec_type"`
			OrdRefID  string  `json:"ord_ref_id"`
			ExecID    string  `json:"exec_id"`
			TradeID   int64   `json:"trade_id"`
			Timestamp string  `json:"timestamp"`
			LastQty   float64 `json:"last_qty"`
			LastPrice float64 `json:"last_price"`
			CumQty    float64 `json:"cum_qty"`
			OrderQty  float64 `json:"order_qty"`
			Fee       float64 `json:"fee"`
			FeeUSD    float64 `json:"fee_usd_equiv"`
			FeeCcy    string  `json:"fee_ccy"`
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
			ClOrdID:   row.ClOrdID,
			Symbol:    row.Symbol,
			Side:      row.Side,
			OrderType: row.OrderType,
			ExecType:  row.ExecType,
			OrdRefID:  row.OrdRefID,
			ExecID:    row.ExecID,
			TradeID:   row.TradeID,
			Timestamp: row.Timestamp,
			LastQty:   row.LastQty,
			LastPrice: row.LastPrice,
			CumQty:    row.CumQty,
			OrderQty:  row.OrderQty,
			Fee:       row.Fee,
			FeeUSD:    row.FeeUSD,
			FeeCcy:    row.FeeCcy,
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
