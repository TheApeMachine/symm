package liquidity

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the liquidity measuring instrument. It composes its market entities
in its constructor and exposes the canonical signal structure: Constructor,
Name, Error, Step, Close. It satisfies nomagique/runtime.Node[*types.Envelope],
writing its projected Measurement into the envelope's Liquidity field.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
}

// NewSignal composes the Ticker (touch) entity. Full-book morphology is added
// as a further entity in a later pass.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "liquidity" }

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
	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	measurement := signal.ticker.Step(envelope.TickerData)

	if measurement != nil {
		signal.err = measurement.Err
	}

	envelope.Liquidity = measurement

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}

type liquidityState struct {
	graph *Graph
	at    time.Time
}

// Ticker owns one causal Primitive liquidity model per symbol and serializes
// each delivered observation with that model's exact event-time coordinate.
type Ticker struct {
	states     map[string]*liquidityState
	projection *data.Projection
	finite     core.Primitive
}

func NewTicker() *Ticker {
	return &Ticker{states: make(map[string]*liquidityState), projection: liquidityProjection(), finite: logic.NewFinite()}
}

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Bid == nil || event.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}
	bid, ask := event.Bid.Float64(), event.Ask.Float64()

	for _, value := range []float64{bid, ask, event.BidQty, event.AskQty, bid * event.BidQty, ask * event.AskQty} {
		if !finiteHolds(ticker.finite, value) || value <= 0 {
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
	fieldsEval := transport.NewEvaluate(state.graph)
	var fields data.ProjectionInput

	for out := range fieldsEval.Next(transport.NewValues(GraphInput{
		BestBid: bid, BestAsk: ask, BidQty: event.BidQty, AskQty: event.AskQty, At: event.Timestamp.UnixNano(),
	}).Next(nil)) {
		fields = *(*data.ProjectionInput)(out)
	}

	err := fieldsEval.Error()
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	state.at = event.Timestamp
	ticker.projection.Identity = func() (string, string, time.Time, time.Time) {
		return fmt.Sprintf("liquidity:%s:%d", event.Symbol, event.Timestamp.UnixNano()), event.Symbol, event.Timestamp, event.Timestamp
	}
	resultEval := transport.NewEvaluate(ticker.projection)
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
func (ticker *Ticker) Close() error { return nil }

/*
finiteHolds reports whether one value passes the Finite primitive.
*/
func finiteHolds(finite core.Primitive, value float64) bool {
	holdsEval := transport.NewEvaluate(finite)
	var holds bool

	for out := range holdsEval.Next(transport.NewValues(value).Next(nil)) {
		holds = *(*bool)(out)
	}

	return holdsEval.Error() == nil && holds
}
