package toxicity

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
Signal is the book-touch liquidity-disposition instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's TypeID:
a Level3 envelope updates the retained per-symbol touch and projects the
book-touch disposition measurement; a Trade envelope matches against that
symbol's last retained touch to attribute fill.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	level3 *Level3
	trade  *Trade
}

/*
NewSignal composes the Level3 (book-touch) and Trade (executed-flow) entities.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		level3: NewLevel3(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "toxicity" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	switch envelope.TypeID {
	case types.EnvelopeLevel3:
		if envelope.Level3Data.Bids == nil && envelope.Level3Data.Asks == nil {
			return envelope
		}

		envelope.Toxicity = signal.StepLevel3(envelope.Level3Data)
	case types.EnvelopeTrade:
		envelope.Toxicity = signal.StepTrade(envelope.TradeData)
	}

	return envelope
}

func (signal *Signal) StepLevel3(message kraken.Level3Data) *data.Measurement[float64] {
	measurement := signal.level3.Step(message)

	if measurement != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = map[string]string{}
		}

		// README §11.1: preserve the attribution source. This entity observes
		// the book touch only, so the attribution is always touch-only
		// bracketing; full-book previous-level observation would be recorded
		// here when a full-book feed supplies Q_1(P_0).
		measurement.Provenance["previous_level_disposition"] = "touch_only_bracketing"
	}

	return measurement
}

func (signal *Signal) StepTrade(tick kraken.TradeData) *data.Measurement[float64] {
	bidPrice, askPrice, bidQty, askQty, found := signal.level3.Touch(tick.Symbol)

	if !found {
		return nil
	}

	return signal.trade.Step(tick, bidPrice, askPrice, bidQty, askQty)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.level3.Close(); err != nil {
		return err
	}

	return signal.trade.Close()
}

type tradeState struct {
	graph              *TradeGraph
	bracketQty         float64
	matchedBidQty      float64
	matchedAskQty      float64
	lastSec            float64
	lastNsec           float64
	prevSec            float64
	prevNsec           float64
	hasTime            bool
	hasPrevTime        bool
	bidFractionSamples int
	askFractionSamples int
}

/*
Trade matches incoming trades against the symbol's retained book touch.
It owns one Primitive graph per symbol.
*/
type Trade struct {
	states     map[string]*tradeState
	symbol     string
	at         time.Time
	projection *data.Projection
	finite     core.Primitive
}

/*
NewTrade constructs the Trade entity with per-symbol Primitive compositions.
*/
func NewTrade() *Trade {
	entity := &Trade{states: make(map[string]*tradeState), projection: tradeProjection(), finite: logic.NewFinite()}
	entity.projection.Identity = entity.identity
	return entity
}

func (trade *Trade) Close() error { return nil }

/*
Step matches one trade against the given touch and projects the fill attribution.
*/
func (trade *Trade) Step(tick kraken.TradeData, bidPrice, askPrice, bidQty, askQty float64) *data.Measurement[float64] {
	if bidPrice == 0 || askPrice == 0 {
		return nil
	}

	for _, value := range []float64{tick.Price.Float64(), tick.Qty, bidPrice, askPrice, bidQty, askQty} {
		if !finiteHolds(trade.finite, value) || value < 0 {
			return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: finite non-negative trade and touch values required")}
		}
	}
	if tick.Qty <= 0 || tick.Price.Sign() <= 0 || (tick.Side != "buy" && tick.Side != "sell") {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: positive execution and known aggressor side required")}
	}
	sec := float64(tick.Timestamp.Unix())
	nsec := float64(tick.Timestamp.Nanosecond())

	state, found := trade.states[tick.Symbol]

	if !found {
		state = &tradeState{graph: newTradeGraph()}
		trade.states[tick.Symbol] = state
	}

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			return nil
		}
	}

	if state.hasTime {
		state.prevSec = state.lastSec
		state.prevNsec = state.lastNsec
		state.hasPrevTime = true
	}

	if !state.hasTime {
		state.prevSec = sec
		state.prevNsec = nsec
		state.hasPrevTime = false
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	tradePrice := tick.Price.Float64()
	tradeQty := tick.Qty

	state.bracketQty += tradeQty

	if tick.Side == "sell" && tradePrice == bidPrice {
		state.matchedBidQty += tradeQty
	}

	if tick.Side == "buy" && tradePrice == askPrice {
		state.matchedAskQty += tradeQty
	}

	bidFillQty := state.matchedBidQty
	askFillQty := state.matchedAskQty

	bidFillFraction := 0.0

	if bidQty > 0 {
		bidFillFraction = bidFillQty / bidQty
	}

	askFillFraction := 0.0

	if askQty > 0 {
		askFillFraction = askFillQty / askQty
	}

	trade.symbol = tick.Symbol
	trade.at = tick.Timestamp

	input := TradeInput{
		BracketQty: state.bracketQty, MatchedBidQty: state.matchedBidQty, MatchedAskQty: state.matchedAskQty,
		TouchFillBidQty: bidFillQty, TouchFillAskQty: askFillQty,
		TouchFillBidFrac: bidFillFraction, TouchFillAskFrac: askFillFraction,
	}
	deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

	if state.hasPrevTime && deltaT > 0 {
		input.TouchFillBidRate = bidFillQty / deltaT
		input.TouchFillAskRate = askFillQty / deltaT
		input.HasRate = true
	}

	if bidFillFraction > 0 {
		state.bidFractionSamples++
	}

	if askFillFraction > 0 {
		state.askFractionSamples++
	}

	input.BidSupported = state.bidFractionSamples >= 3
	input.AskSupported = state.askFractionSamples >= 3
	fieldsEval := transport.NewEvaluate(state.graph)
	var fields data.ProjectionInput

	for out := range fieldsEval.Next(transport.NewValues(input).Next(nil)) {
		fields = *(*data.ProjectionInput)(out)
	}

	err := fieldsEval.Error()
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	resultEval := transport.NewEvaluate(trade.projection)
	var measurement *data.Measurement[float64]

	for out := range resultEval.Next(transport.NewValues(fields).Next(nil)) {
		measurement = *(**data.Measurement[float64])(out)
	}

	err = resultEval.Error()

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	return measurement
}

func (trade *Trade) identity() (string, string, time.Time, time.Time) {
	return fmt.Sprintf("toxicity:trade:%s:%d", trade.symbol, trade.at.UnixNano()),
		trade.symbol,
		trade.at,
		trade.at
}

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
