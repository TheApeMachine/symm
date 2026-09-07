package liquidity

import (
	"fmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"sync"
	"time"
)

type liquidityState struct {
	graph core.Primitive
	at    time.Time
}

// Ticker owns one causal Primitive liquidity model per symbol and serializes
// each delivered observation with that model's exact event-time coordinate.
type Ticker struct {
	mu         sync.Mutex
	states     map[string]*liquidityState
	projection *data.Projection
}

func NewTicker() *Ticker {
	return &Ticker{states: make(map[string]*liquidityState), projection: liquidityProjection()}
}

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Bid == nil || event.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}
	bid, ask := event.Bid.Float64(), event.Ask.Float64()
	for _, value := range []float64{bid, ask, event.BidQty, event.AskQty, bid * event.BidQty, ask * event.AskQty} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: finite positive prices and displayed quantities required")}
		}
	}
	if ask <= bid {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: positive order violated (%f <= %f)", ask, bid)}
	}
	ticker.mu.Lock()
	defer ticker.mu.Unlock()
	state := ticker.states[event.Symbol]
	if state == nil {
		state = &liquidityState{graph: newLiquidityGraph()}
		ticker.states[event.Symbol] = state
	} else if event.Timestamp.Before(state.at) {
		return nil
	}
	fields, err := transport.Evaluate[map[string]core.Primitive](state.graph, core.Record(map[string]any{
		"best_bid_price": bid, "best_ask_price": ask, "touch_quantity:bid": event.BidQty, "touch_quantity:ask": event.AskQty, "at": event.Timestamp.UnixNano(),
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
