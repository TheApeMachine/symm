package order

import (
	"encoding/json"
	"fmt"
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
ParseExecutionFills extracts trade fills from a Kraken v2 executions frame.
*/
func ParseExecutionFills(payload []byte) ([]Fill, error) {
	var frame struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
		Data    []struct {
			OrderID  string  `json:"order_id"`
			Symbol   string  `json:"symbol"`
			Side     string  `json:"side"`
			ExecType string  `json:"exec_type"`
			LastQty  float64 `json:"last_qty"`
			LastPx   float64 `json:"last_price"`
		} `json:"data"`
	}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal executions frame: %w", err)
	}

	if frame.Channel != "executions" {
		return nil, fmt.Errorf("not an executions frame: %q", frame.Channel)
	}

	fills := make([]Fill, 0, len(frame.Data))

	for _, row := range frame.Data {
		if row.ExecType != "trade" {
			continue
		}

		if row.LastQty <= 0 || row.LastPx <= 0 {
			continue
		}

		fills = append(fills, Fill{
			OrderID: row.OrderID,
			Symbol:  row.Symbol,
			Side:    row.Side,
			Qty:     row.LastQty,
			Price:   row.LastPx,
		})
	}

	return fills, nil
}
