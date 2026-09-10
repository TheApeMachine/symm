package liquidity

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

type liquidityState struct {
	graph *Graph
	at    time.Time
}

// Ticker owns one causal Primitive liquidity model per symbol and serializes
// each delivered observation with that model's exact event-time coordinate.
type Ticker struct {
	states     map[string]*liquidityState
	projection *data.Projection
	finite     *logic.Finite[float64]
}

func NewTicker() *Ticker {
	return &Ticker{states: make(map[string]*liquidityState), projection: liquidityProjection(), finite: logic.NewFinite[float64]()}
}

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Bid == nil || event.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}
	bid, ask := event.Bid.Float64(), event.Ask.Float64()

	for _, value := range []float64{bid, ask, event.BidQty, event.AskQty, bid * event.BidQty, ask * event.AskQty} {
		if !ticker.finite.Holds(value) || value <= 0 {
			return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: finite positive prices and displayed quantities required")}
		}
	}
	if ask <= bid {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: positive order violated (%f <= %f)", ask, bid)}
	}

	state := ticker.states[event.Symbol]
	if state == nil {
		state = &liquidityState{graph: newLiquidityGraph()}
		ticker.states[event.Symbol] = state
	} else if event.Timestamp.Before(state.at) {
		return nil
	}
	fields, err := transport.Evaluate(state.graph, transport.Values(GraphInput{
		BestBid: bid, BestAsk: ask, BidQty: event.BidQty, AskQty: event.AskQty, At: event.Timestamp.UnixNano(),
	}))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	state.at = event.Timestamp
	ticker.projection.Identity = func() (string, string, time.Time, time.Time) {
		return fmt.Sprintf("liquidity:%s:%d", event.Symbol, event.Timestamp.UnixNano()), event.Symbol, event.Timestamp, event.Timestamp
	}
	return ticker.projection.Project(fields)
}
func (ticker *Ticker) Close() error { return nil }
