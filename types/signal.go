package types

import "github.com/theapemachine/symm/kraken"

/*
Signal consumes ordered market data on the ingress channels it exposes and emits
measurement batches on the channel returned by Measure, so the runtime can fan
every signal in concurrently. Ack reports that one ingress frame finished so the
runtime can barrier before draining measurements.
*/
type Signal interface {
	Tickers() chan []kraken.TickerData
	Books() chan []kraken.BookData
	Trades() chan []kraken.TradeData
	Ack() <-chan struct{}
	Measure() chan []*Measurement
	Close() error
}
