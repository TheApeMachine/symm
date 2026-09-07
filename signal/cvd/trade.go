package cvd

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

var errUnmeasurable = errors.New("cvd: trade requires a finite positive price and quantity and a known aggressor side")

type flowState struct {
	graph    core.Primitive
	from, at time.Time
	priorMid float64
}

// Trade owns per-symbol event chronology and one retained execution graph per
// symbol. The graph owns accumulation and estimator state, not mutable slots.
type Trade struct {
	mutex      sync.Mutex
	states     map[string]*flowState
	projection *data.Projection
	quote      func(string) (*decimal.Decimal, *decimal.Decimal)
}

func NewTrade() *Trade {
	return &Trade{states: make(map[string]*flowState), projection: flowProjection()}
}
func (trade *Trade) SetQuote(quote func(string) (*decimal.Decimal, *decimal.Decimal)) {
	trade.mutex.Lock()
	defer trade.mutex.Unlock()
	trade.quote = quote
}

// Step delivers one validated market observation and drains its complete
// Primitive run. Quotes do not become response evidence until a prior exists.
func (trade *Trade) Step(event kraken.TradeData) *data.Measurement[float64] {
	price, quantity := event.Price.Float64(), event.Qty
	if price <= 0 || quantity <= 0 || math.IsNaN(price) || math.IsInf(price, 0) || math.IsNaN(quantity) || math.IsInf(quantity, 0) || math.IsInf(price*quantity, 0) || (event.Side != "buy" && event.Side != "sell") {
		return &data.Measurement[float64]{Err: errUnmeasurable}
	}
	trade.mutex.Lock()
	defer trade.mutex.Unlock()
	state := trade.states[event.Symbol]
	if state == nil {
		state = &flowState{graph: newFlowGraph(), from: event.Timestamp}
		trade.states[event.Symbol] = state
	} else if event.Timestamp.Before(state.at) {
		return nil
	}
	midpoint := 0.0
	if trade.quote != nil {
		bid, ask := trade.quote(event.Symbol)
		if bid != nil && ask != nil && bid.Sign() > 0 && ask.Cmp(bid) >= 0 {
			midpoint = (bid.Float64() + ask.Float64()) / 2
		}
	}
	fields, err := transport.Evaluate[map[string]core.Primitive](state.graph, core.Record(map[string]any{
		"price": price, "quantity": quantity, "buy": event.Side == "buy", "at": event.Timestamp.UnixNano(), "from": state.from.UnixNano(),
		"midpoint": midpoint, "prior_mid": state.priorMid, "quoted": midpoint > 0 && state.priorMid > 0,
	}))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	state.at = event.Timestamp
	if midpoint > 0 {
		state.priorMid = midpoint
	}
	trade.projection.Identity = func() (string, string, time.Time, time.Time) {
		return event.Symbol + ":cvd:" + event.Timestamp.Format(time.RFC3339Nano), event.Symbol, event.Timestamp, state.from
	}
	return trade.projection.Project(fields)
}
func (trade *Trade) Close() error { return nil }
