package toxicity

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type tradeState struct {
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

type tradeInput struct {
	bracketQty       calculus.Constant
	matchedBidQty    calculus.Constant
	matchedAskQty    calculus.Constant
	touchFillBidQty  calculus.Constant
	touchFillAskQty  calculus.Constant
	touchFillBidFrac calculus.Constant
	touchFillAskFrac calculus.Constant
	touchFillBidRate calculus.Constant
	touchFillAskRate calculus.Constant
	hasRate          calculus.Constant
}

/*
Trade matches incoming trades against the symbol's retained book touch.
It maintains a single inlined nomagique.Number composition.
*/
type Trade struct {
	mu     sync.Mutex
	number *nomagique.Pipeline

	states map[string]*tradeState
	symbol string
	at     time.Time

	bidStd *equation.CausalResidual
	askStd *equation.CausalResidual

	in tradeInput
}

/*
NewTrade constructs the Trade entity with a single inlined Number composition.
*/
func NewTrade() *Trade {
	trade := &Trade{
		states: make(map[string]*tradeState),
	}

	keyFn := func() string { return trade.symbol }
	trade.bidStd = &equation.CausalResidual{Key: keyFn}
	trade.askStd = &equation.CausalResidual{Key: keyFn}

	in := &trade.in

	trade.number = nomagique.Number(&nmtypes.Chain{
		A: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "bracket_trade_quantity", Unit: "count", Timescale: "instantaneous",
				Value: &in.bracketQty,
			},
			B: &nmtypes.Report{
				Label: "matched_touch_trade_quantity:bid", Unit: "count", Timescale: "instantaneous",
				Value: &in.matchedBidQty,
			},
			C: &nmtypes.Report{
				Label: "matched_touch_trade_quantity:ask", Unit: "count", Timescale: "instantaneous",
				Value: &in.matchedAskQty,
			},
			D: &nmtypes.Report{
				Label: "touch_fill_quantity:bid", Unit: "count", Timescale: "instantaneous",
				Value: &in.touchFillBidQty,
			},
		},
		B: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "touch_fill_quantity:ask", Unit: "count", Timescale: "instantaneous",
				Value: &in.touchFillAskQty,
			},
			B: &nmtypes.Report{
				Label: "touch_fill_fraction:bid", Unit: "dimensionless", Timescale: "instantaneous",
				Value: &in.touchFillBidFrac,
			},
			C: &nmtypes.Report{
				Label: "touch_fill_fraction:ask", Unit: "dimensionless", Timescale: "instantaneous",
				Value: &in.touchFillAskFrac,
			},
		},
		C: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "touch_fill_rate:bid", Unit: "per_second", Timescale: "per_second",
				Value: &in.touchFillBidRate, Defined: &in.hasRate,
			},
			B: &nmtypes.Report{
				Label: "touch_fill_rate:ask", Unit: "per_second", Timescale: "per_second",
				Value: &in.touchFillAskRate, Defined: &in.hasRate,
			},
			C: &nmtypes.Chain{
				A: &in.touchFillBidFrac,
				B: &nmtypes.Labelled{
					Prefix: "fill_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"baseline":   "baseline:bid",
							"divergence": "divergence:bid",
							"zscore":     "zscore:bid",
						},
						Node: trade.bidStd,
					},
				},
			},
			D: &nmtypes.Chain{
				A: &in.touchFillAskFrac,
				B: &nmtypes.Labelled{
					Prefix: "fill_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"baseline":   "baseline:ask",
							"divergence": "divergence:ask",
							"zscore":     "zscore:ask",
						},
						Node: trade.askStd,
					},
				},
			},
		},
		D: &data.Projection{
			Source:   "toxicity",
			Identity: trade.identity,
		},
	})

	return trade
}

func (trade *Trade) Close() error { return nil }

/*
Step matches one trade against the given touch and projects the fill attribution.
*/
func (trade *Trade) Step(tick kraken.TradeData, bidPrice, askPrice, bidQty, askQty float64) *data.Measurement[float64] {
	if bidPrice == 0 || askPrice == 0 {
		return nil
	}

	sec := float64(tick.Timestamp.Unix())
	nsec := float64(tick.Timestamp.Nanosecond())

	trade.mu.Lock()
	defer trade.mu.Unlock()

	state, found := trade.states[tick.Symbol]

	if !found {
		state = &tradeState{}
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

	in := &trade.in
	in.bracketQty.Value = nmtypes.Number(state.bracketQty)
	in.matchedBidQty.Value = nmtypes.Number(state.matchedBidQty)
	in.matchedAskQty.Value = nmtypes.Number(state.matchedAskQty)
	in.touchFillBidQty.Value = nmtypes.Number(bidFillQty)
	in.touchFillAskQty.Value = nmtypes.Number(askFillQty)
	in.touchFillBidFrac.Value = nmtypes.Number(bidFillFraction)
	in.touchFillAskFrac.Value = nmtypes.Number(askFillFraction)

	deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

	if state.hasPrevTime && deltaT > 0 {
		in.touchFillBidRate.Value = nmtypes.Number(bidFillQty / deltaT)
		in.touchFillAskRate.Value = nmtypes.Number(askFillQty / deltaT)
		in.hasRate.Value = 1
	} else {
		in.hasRate.Value = 0
	}

	if bidFillFraction > 0 {
		state.bidFractionSamples++
	}

	if askFillFraction > 0 {
		state.askFractionSamples++
	}

	trade.number.Step(1.0)
	measurement := trade.number.Measurement()

	if measurement != nil {
		if state.bidFractionSamples >= 3 {
			measurement.SNRDefined = true
			measurement.SNR = math.Abs(float64(trade.bidStd.ZScore()))
		} else if state.askFractionSamples >= 3 {
			measurement.SNRDefined = true
			measurement.SNR = math.Abs(float64(trade.askStd.ZScore()))
		} else {
			measurement.SNRDefined = false
			measurement.SNR = 0
		}
	}

	return measurement
}

func (trade *Trade) identity() (string, string, time.Time, time.Time) {
	return fmt.Sprintf("toxicity:trade:%s:%d", trade.symbol, trade.at.UnixNano()),
		trade.symbol,
		trade.at,
		trade.at
}
