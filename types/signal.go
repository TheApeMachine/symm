package types

import "github.com/theapemachine/symm/kraken"

/*
Signal receives market rows on channels and yields measurements via Drain.
Ack yields one value after each ingress frame finishes Measure so Crypto can
barrier before draining outs.
*/
type Signal interface {
	Tickers() chan []kraken.TickerData
	Books() chan []kraken.BookData
	Trades() chan []kraken.TradeData
	Measure() chan []*Measurement
	Ack() <-chan struct{}
}
