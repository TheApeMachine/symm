package derivatives

import (
	"errors"
	"iter"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
liquidationState is one symbol's liquidation accounting: its cumulative
totals, the earliest through latest accounted event time, and the previous
share the velocity is measured against.
*/
type liquidationState struct {
	clock map[string]time.Time

	liqBuyTotal      float64
	liqSellTotal     float64
	grossTradeTotal  float64
	startTime        time.Time
	tradeCount       float64
	prevLiqShare     float64
	hasPrevLiqShare  bool
	lastAdvancedTime time.Time
}

/*
Liquidation owns the liquidation-notional accounting for every symbol. It
measures gross and net liquidation flow, liquidation share of aggregate
volume, and throughput rates across a causal timeline, writing every metric
into the measurement where it computes it.
*/
type Liquidation struct {
	err    error
	states map[string]*liquidationState
}

func NewLiquidation() core.Primitive {
	return &Liquidation{states: make(map[string]*liquidationState)}
}

func (op *Liquidation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			state, existing := op.states[m.Label]

			if !existing {
				state = &liquidationState{clock: make(map[string]time.Time)}
				op.states[m.Label] = state
			}

			op.observe(m, state, existing)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Liquidation) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
observe accounts one trade into the symbol's cumulative state. Totals include
every received trade, including historical reconnect data; late trades revise
totals and the earliest boundary, but do not emit a rate or share change until
the live event-time clock advances again.
*/
func (op *Liquidation) observe(m *data.Measurement[float64], state *liquidationState, existing bool) {
	stamped, advanced := stamp(state.clock, m.Label, m.At, func() bool { v, _ := m.GetProvenance("synthetic_timestamp"); return v == "true" }())

	m.EnsureMetadata()

	price := m.GetMetric("price").Raw
	quantity := m.GetMetric("qty").Raw
	notional := price * quantity

	if !existing {
		state.startTime = stamped
	}

	state.grossTradeTotal += notional

	typ, _ := m.GetProvenance("type")
	if typ == "liquidation" {
		side, _ := m.GetProvenance("side")
	switch side {
		case "buy":
			state.liqBuyTotal += notional
		case "sell":
			state.liqSellTotal += notional
		}
	}

	if stamped.Before(state.startTime) {
		state.startTime = stamped
	}

	if advanced {
		state.tradeCount++
		state.lastAdvancedTime = stamped
	}

	grossLiq := state.liqBuyTotal + state.liqSellTotal
	netLiq := state.liqBuyTotal - state.liqSellTotal

	m.From = state.startTime
	m.At = state.lastAdvancedTime

	m.WriteMetric("liquidation_notional:buy", state.liqBuyTotal)
	m.WriteMetric("liquidation_notional:sell", state.liqSellTotal)
	m.WriteMetric("gross_liquidation_notional", grossLiq)
	m.WriteMetric("net_liquidation_notional", netLiq)
	m.WriteMetric("gross_derivative_trade_notional", state.grossTradeTotal)

	var currentShare float64

	if grossLiq > 0 {
		m.WriteNormalized("liquidation_signed_fraction", netLiq/grossLiq)
	}

	if state.grossTradeTotal > 0 {
		currentShare = grossLiq / state.grossTradeTotal
		m.WriteNormalized("liquidation_share", currentShare)
	}

	if advanced {
		duration := state.lastAdvancedTime.Sub(state.startTime).Seconds()

		if duration > 0 {
			m.WriteMetric("liquidation_notional_rate", grossLiq / duration)
		}

		if state.hasPrevLiqShare {
			m.WriteMetric("liquidation_share_velocity", currentShare - state.prevLiqShare)
		}

		state.prevLiqShare = currentShare
		state.hasPrevLiqShare = true
	}

	m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(state.tradeCount, 'f', -1, 64))
}
