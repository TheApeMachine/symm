package trade

import "time"

type Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
}

type Subscribe struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id,omitempty"`
}

/*
NewSubscribe builds a Kraken v2 trade channel subscription request.
*/
func NewSubscribe(symbols []string) *Subscribe {
	return &Subscribe{
		Method: "subscribe",
		Params: Params{
			Channel:  "trade",
			Symbol:   symbols,
			Snapshot: true,
		},
		ReqID: int(time.Now().UnixNano()),
	}
}
