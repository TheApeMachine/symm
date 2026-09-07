package toxicity

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tradeState struct {
	graph              core.Primitive
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
}

/*
NewTrade constructs the Trade entity with per-symbol Primitive compositions.
*/
func NewTrade() *Trade {
	entity := &Trade{states: make(map[string]*tradeState), projection: tradeProjection()}
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
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
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
	} else {
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

	input := make(map[string]any)
	input["bracketQty"] = state.bracketQty
	input["matchedBidQty"] = state.matchedBidQty
	input["matchedAskQty"] = state.matchedAskQty
	input["touchFillBidQty"] = bidFillQty
	input["touchFillAskQty"] = askFillQty
	input["touchFillBidFrac"] = bidFillFraction
	input["touchFillAskFrac"] = askFillFraction

	deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

	if state.hasPrevTime && deltaT > 0 {
		input["touchFillBidRate"] = bidFillQty / deltaT
		input["touchFillAskRate"] = askFillQty / deltaT
		input["hasRate"] = true
	} else {
		input["hasRate"] = false
	}

	if bidFillFraction > 0 {
		state.bidFractionSamples++
	}

	if askFillFraction > 0 {
		state.askFractionSamples++
	}

	input["bidSupported"] = state.bidFractionSamples >= 3
	input["askSupported"] = state.askFractionSamples >= 3
	fields, err := transport.Evaluate[map[string]core.Primitive](state.graph, core.Record(input))
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
