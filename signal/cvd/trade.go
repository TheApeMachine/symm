package cvd

import (
	"context"
	"errors"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
errUnmeasurable rejects observations the Finite primitive cannot score.
*/
var errUnmeasurable = errors.New("cvd: trade requires a finite positive price and quantity and a known aggressor side")

/*
Signal is the CVD executed-flow measuring instrument. It composes its market
entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	trade  *Trade
}

/*
NewSignal composes the Trade (executed-flow) entity. quote, when non-nil,
supplies the contemporaneous top-of-book bid/ask so the response-price metrics
(midpoint and midpoint_log_return) can be computed; without it they remain
permanently undefined and only the executed-flow accounting is measured.
*/
func NewSignal(ctx context.Context, quote func(symbol string) (bid, ask *decimal.Decimal)) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	trade := NewTrade()
	trade.SetQuote(quote)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trade:  trade,
	}
}

func (signal *Signal) Name() string { return "cvd" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeTrade {
		return envelope
	}

	envelope.CVD = signal.trade.Step(envelope.TradeData)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}

type flowState struct {
	graph    *Flow
	from, at time.Time
	priorMid float64
}

// Trade owns per-symbol event chronology and one retained execution graph per
// symbol. The graph owns accumulation and estimator state, not mutable slots.
type Trade struct {
	states     map[string]*flowState
	projection *data.Projection
	quote      func(string) (*decimal.Decimal, *decimal.Decimal)
	finite     core.Primitive
}

func NewTrade() *Trade {
	return &Trade{
		states:     make(map[string]*flowState),
		projection: flowProjection(),
		finite:     logic.NewFinite(),
	}
}

func (trade *Trade) SetQuote(quote func(string) (*decimal.Decimal, *decimal.Decimal)) {
	trade.quote = quote
}

// Step delivers one validated market observation and drains its complete
// Primitive run. Quotes do not become response evidence until a prior exists.
func (trade *Trade) Step(event kraken.TradeData) *data.Measurement[float64] {
	price, quantity := event.Price.Float64(), event.Qty

	if !finiteHolds(trade.finite, price, quantity, price*quantity) ||
		price <= 0 || quantity <= 0 || (event.Side != "buy" && event.Side != "sell") {
		return &data.Measurement[float64]{Err: errUnmeasurable}
	}

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

	fieldsEval := transport.NewEvaluate(state.graph)
	var fields data.ProjectionInput

	for out := range fieldsEval.Next(transport.NewValues(FlowInput{
		Price: price, Quantity: quantity, Buy: event.Side == "buy",
		At: event.Timestamp.UnixNano(), From: state.from.UnixNano(),
		Midpoint: midpoint, PriorMid: state.priorMid,
		Quoted: midpoint > 0 && state.priorMid > 0,
	}).Next(nil)) {
		fields = *(*data.ProjectionInput)(out)
	}

	err := fieldsEval.Error()
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	state.at = event.Timestamp

	if midpoint > 0 {
		state.priorMid = midpoint
	}

	trade.projection.Identity = func() (string, time.Time, time.Time) {
		return event.Symbol, event.Timestamp, state.from
	}

	resultEval := transport.NewEvaluate(trade.projection)
	var result *data.Measurement[float64]

	for out := range resultEval.Next(transport.NewValues(fields).Next(nil)) {
		result = *(**data.Measurement[float64])(out)
	}

	err = resultEval.Error()

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	return result
}

func (trade *Trade) Close() error { return nil }

/*
finiteHolds reports whether every value passes the Finite primitive.
*/
func finiteHolds(finite core.Primitive, values ...float64) bool {
	for _, value := range values {
		holdsEval := transport.NewEvaluate(finite)
		var holds bool

		for out := range holdsEval.Next(transport.NewValues(value).Next(nil)) {
			holds = *(*bool)(out)
		}

		if holdsEval.Error() != nil || !holds {
			return false
		}
	}

	return true
}
