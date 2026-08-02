package tests

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
)

const (
	defaultPaperQuoteBalance = 200.00
	defaultPaperMakerFee     = 0.0016
	defaultPaperTakerFee     = 0.0026
)

/*
Market ranges ready fixture payloads into the fake Kraken connections.
*/
type Market struct {
	ctx     context.Context
	cancel  context.CancelFunc
	Public  *Conn
	Private *Conn
	Level3  *Conn
	Symbols []string
}

/*
NewMarket creates a simulated market with the given number of symbols.
It replaces the production Kraken API WebSockets and REST routes with
in-memory fixtures that emit deterministic events for testing.
This allows the full system to be exercised without relying on the
live Kraken API, which not only helps with testing the system mechanics,
but also the strategy and logic.
*/
func NewMarket(
	ctx context.Context,
	symbolCount int,
) *Market {
	if symbolCount < 1 {
		panic(errnie.Err(errnie.Validation, "tests: symbol count must be positive", nil))
	}

	ctx, cancel := context.WithCancel(ctx)
	symbols := make([]string, symbolCount)

	for index := range symbolCount {
		symbols[index] = fmt.Sprintf("SIM%d/USD", index+1)
	}

	market := &Market{
		ctx:     ctx,
		cancel:  cancel,
		Public:  NewConn(ctx),
		Private: NewConn(ctx),
		Level3:  NewConn(ctx),
		Symbols: symbols,
	}

	return market
}

/*
Sequence is a convenience method to replay a series of market states.
*/
func (market *Market) Sequence(states ...*MarketState) {

}

/*
Close releases the four in-memory connections and simulated market context.
*/
func (market *Market) Close() {
	market.Public.Close()
	market.Private.Close()
	market.Level3.Close()
	market.cancel()
}
