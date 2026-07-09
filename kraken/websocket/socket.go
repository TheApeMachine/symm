package websocket

import "github.com/theapemachine/symm/kraken"

type Socket interface {
	Observe(string) chan []byte
}

type PublicSocket interface {
	Socket
	Ticker([]string) (kraken.TickerDataSlice, error)
}
