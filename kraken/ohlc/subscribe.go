package ohlc

import "time"

type Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Interval int      `json:"interval"`
	Snapshot bool     `json:"snapshot"`
}

type Subscribe struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id,omitempty"`
}

func NewSubscribe(symbols []string) *Subscribe {
	return &Subscribe{
		Method: "subscribe",
		Params: Params{
			Channel:  "ohlc",
			Symbol:   symbols,
			Interval: 1,
			Snapshot: true,
		},
		ReqID: int(time.Now().UnixNano()),
	}
}
