package order

import (
	"encoding/json"
	"fmt"
)

/*
Ack is the Kraken WebSocket v2 trading method response envelope.
*/
type Ack struct {
	Method  string `json:"method"`
	ReqID   int    `json:"req_id"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Result  struct {
		OrderID      string `json:"order_id"`
		ClOrdID      string `json:"cl_ord_id"`
		OrderUserref int    `json:"order_userref"`
	} `json:"result"`
}

/*
ParseAck unmarshals one trading method response frame.
*/
func ParseAck(payload []byte) (*Ack, error) {
	var ack Ack

	if err := json.Unmarshal(payload, &ack); err != nil {
		return nil, fmt.Errorf("unmarshal order ack: %w", err)
	}

	if ack.Method == "" {
		return nil, fmt.Errorf("missing method in order ack")
	}

	return &ack, nil
}
