package types

import "github.com/theapemachine/symm/kraken"

/*
Feed drains queued market frames and returns measurements from composed signals.
*/
type Feed interface {
	StatusReporter
	On([]byte)
	Measure() ([]*Measurement, error)
}

/*
MarketFeed exposes the explicit public-market subscriptions components consume
directly.
*/
type MarketFeed interface {
	Ticker() *Subscription[*kraken.Ticker]
	Book() *Subscription[*kraken.Book]
	Trade() *Subscription[*kraken.Trade]
	Instrument() *Subscription[*kraken.Instrument]
}

/*
AccountFeed exposes the explicit private-account subscriptions components
consume directly.
*/
type AccountFeed interface {
	Balances() *Subscription[[]byte]
	Executions() *Subscription[[]byte]
	Orders() *Subscription[[]byte]
}
