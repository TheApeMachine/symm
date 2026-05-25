package ohlc

import "time"

type Data struct {
	Symbol        string    `json:"symbol"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	VWAP          float64   `json:"vwap"`
	Volume        float64   `json:"volume"`
	IntervalBegin time.Time `json:"count"`
	Interval      time.Time `json:"interval"`
}

type Snapshot struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    []Data `json:"data"`
}
