package response

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
)

/*
FillNotice is an internal observer payload from Orders to Executions.
*/
type FillNotice struct {
	Symbol       string
	Side         string
	OrderQty     float64
	ClOrdID      string
	OrderType    string
	OrderID      string
	Price        float64
	Fee          float64
	Reason       string
	LiquidityInd string
	Maker        bool
	Partial      bool
}

/*
ArmNotice is an internal observer payload when a protective order rests.
*/
type ArmNotice struct {
	Symbol    string
	Side      string
	OrderQty  float64
	ClOrdID   string
	OrderType string
	OrderID   string
}

func parseAddOrder(message []byte) (map[string]any, bool) {
	var frame struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}

	if sonic.Unmarshal(message, &frame) != nil || frame.Method != "add_order" {
		return nil, false
	}

	var wire map[string]any

	if sonic.Unmarshal(frame.Params, &wire) != nil {
		return nil, false
	}

	return wire, true
}

func fillNoticeFromWire(wire map[string]any) FillNotice {
	notice := FillNotice{
		Symbol:    stringField(wire, "symbol"),
		Side:      stringField(wire, "side"),
		OrderQty:  floatField(wire, "order_qty"),
		ClOrdID:   stringField(wire, "cl_ord_id"),
		OrderType: stringField(wire, "order_type"),
	}

	return notice
}

func fillExecution(notice FillNotice) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	return map[string]any{
		"order_id":      notice.OrderID,
		"cl_ord_id":     notice.ClOrdID,
		"symbol":        notice.Symbol,
		"side":          notice.Side,
		"order_type":    notice.OrderType,
		"order_qty":     notice.OrderQty,
		"order_status":  "filled",
		"exec_type":     "trade",
		"exec_id":       notice.OrderID,
		"last_qty":      notice.OrderQty,
		"last_price":    notice.Price,
		"avg_price":     notice.Price,
		"cum_qty":       notice.OrderQty,
		"liquidity_ind": notice.LiquidityInd,
		"timestamp":     now,
	}
}

func stringField(wire map[string]any, key string) string {
	value, _ := wire[key].(string)

	return value
}

func floatField(wire map[string]any, key string) float64 {
	switch typed := wire[key].(type) {
	case float64:
		return typed
	case json.Number:
		parsed, _ := typed.Float64()

		return parsed
	default:
		return 0
	}
}
