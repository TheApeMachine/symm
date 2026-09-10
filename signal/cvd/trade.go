package cvd

import (
	"errors"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

var errUnmeasurable = errors.New("cvd: trade requires a finite positive price and quantity and a known aggressor side")

type flowState struct {
	graph    *Flow
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
	finite     *logic.Finite[float64]
}

func NewTrade() *Trade {
	return &Trade{
		states:     make(map[string]*flowState),
		projection: flowProjection(),
		finite:     logic.NewFinite[float64](),
	}
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
	priceOK, err := transport.Evaluate(trade.finite, transport.Values(price))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	qtyOK, err := transport.Evaluate(trade.finite, transport.Values(quantity))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	notionalOK, err := transport.Evaluate(trade.finite, transport.Values(price*quantity))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	if !priceOK || !qtyOK || !notionalOK || price <= 0 || quantity <= 0 || (event.Side != "buy" && event.Side != "sell") {
		return &data.Measurement[float64]{Err: errUnmeasurable}
	}

	trade.mutex.Lock()
	defer trade.mutex.Unlock()
	state := trade.states[event.Symbol]
	existing := state != nil

	if !existing {
		state = &flowState{graph: newFlowGraph(), from: event.Timestamp}
		trade.states[event.Symbol] = state
	}

	if existing && event.Timestamp.Before(state.at) {
		return nil
	}

	midpoint := 0.0

	if trade.quote != nil {
		bid, ask := trade.quote(event.Symbol)

		if bid != nil && ask != nil && bid.Sign() > 0 && ask.Cmp(bid) >= 0 {
			midpoint = (bid.Float64() + ask.Float64()) / 2
		}
	}

	fields, err := transport.Evaluate(state.graph, transport.Values(FlowInput{
		Price: price, Quantity: quantity, Buy: event.Side == "buy",
		At: event.Timestamp.UnixNano(), From: state.from.UnixNano(),
		Midpoint: midpoint, PriorMid: state.priorMid,
		Quoted: midpoint > 0 && state.priorMid > 0,
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
