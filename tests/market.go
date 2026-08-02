package tests

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
	Symbols []*Symbol
	State   MarketState
}

/*
NewMarket creates a simulated market with the given number of symbols.
It replaces the production Kraken API WebSockets and REST routes with
in-memory fixtures that emit deterministic events for testing.
*/
func NewMarket(
	ctx context.Context,
	symbols []*Symbol,
) *Market {
	ctx, cancel := context.WithCancel(ctx)

	market := &Market{
		ctx:     ctx,
		cancel:  cancel,
		Public:  NewConn(ctx),
		Private: NewConn(ctx),
		Level3:  NewConn(ctx),
		Symbols: symbols,
		State:   Baseline,
	}

	return market
}

/*
Transition into another market state. This should not be an instance regime
shift, instead it should take a realistic amount of ticks to move from one
state into another. This is meant to simulate an actual market regime shift.
*/
func (market *Market) Transition(state MarketState) {
	market.State = state

	for _, symbol := range market.Symbols {
		symbol.generator.SetState(state, MomentumMap[state])
	}
}

/*
Tick the market. This does not give you any result, because the result should
come from the code that is currently being tested.
*/
func (market *Market) Tick() {
	for _, symbol := range market.Symbols {
		symbol.generator.Step()
	}
}

func (market *Market) Close() {
	market.Public.Close()
	market.Private.Close()
	market.Level3.Close()
	market.cancel()
}

func WithMarket(t *testing.T, symbols []*Symbol, f func(*Market)) func() {
	return func() {
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Reset(func() {
			market.Close()
		})

		f(market)
	}
}
