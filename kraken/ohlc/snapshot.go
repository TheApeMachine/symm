package ohlc

import "time"

/*
Data is one Kraken WebSocket v2 OHLC candle.
See https://docs.kraken.com/api/docs/websocket-v2/ohlc
*/
type Data struct {
	Symbol        string    `json:"symbol"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	VWAP          float64   `json:"vwap"`
	Volume        float64   `json:"volume"`
	IntervalBegin time.Time `json:"interval_begin"`
	Interval      int       `json:"interval"`
}

/*
Snapshot is a Kraken v2 OHLC channel snapshot or update frame.
*/
type Snapshot struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    []Data `json:"data"`
}
