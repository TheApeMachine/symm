package trade

import "time"

/*
Data is one Kraken WebSocket v2 trade execution.
See https://docs.kraken.com/api/docs/websocket-v2/trade
*/
type Data struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	OrdType   string    `json:"ord_type"`
	TradeID   int64     `json:"trade_id"`
	Timestamp time.Time `json:"timestamp"`
}

/*
Snapshot is a Kraken v2 trade channel snapshot or update frame.
*/
type Snapshot struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    []Data `json:"data"`
}
