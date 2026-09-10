package toxicity

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

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
	mu         sync.Mutex
	states     map[string]*tradeState
	symbol     string
	at         time.Time
	projection *data.Projection
	finite     *logic.Finite[float64]
}

/*
NewTrade constructs the Trade entity with per-symbol Primitive compositions.
*/
func NewTrade() *Trade {
	entity := &Trade{states: make(map[string]*tradeState), projection: tradeProjection(), finite: logic.NewFinite[float64]()}
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
		ok, err := transport.Evaluate(trade.finite, transport.Values(value))
		if err != nil {
			return &data.Measurement[float64]{Err: err}
		}
		if !ok || value < 0 {
			return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: finite non-negative trade and touch values required")}
		}
	}
	if tick.Qty <= 0 || tick.Price.Sign() <= 0 || (tick.Side != "buy" && tick.Side != "sell") {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: positive execution and known aggressor side required")}
	}
	sec := float64(tick.Timestamp.Unix())
	nsec := float64(tick.Timestamp.Nanosecond())

	trade.mu.Lock()
	defer trade.mu.Unlock()

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
	fields, err := transport.Evaluate(state.graph, transport.Values(input))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	measurement := trade.projection.Project(fields)

	return measurement
}

func (trade *Trade) identity() (string, string, time.Time, time.Time) {
	return fmt.Sprintf("toxicity:trade:%s:%d", trade.symbol, trade.at.UnixNano()),
		trade.symbol,
		trade.at,
		trade.at
}
