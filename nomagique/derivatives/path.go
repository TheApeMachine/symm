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

	tradeScale := state.grossTradeTotal

	m.SetMetric("liquidation_notional:buy", data.NewMetric[float64](
		"liquidation_notional:buy",
		data.UnitNotional,
		data.TimescaleRollingWindow,
		0.0,
		tradeScale,
	).Write(state.liqBuyTotal))
	m.SetMetric("liquidation_notional:sell", data.NewMetric[float64](
		"liquidation_notional:sell",
		data.UnitNotional,
		data.TimescaleRollingWindow,
		0.0,
		tradeScale,
	).Write(state.liqSellTotal))
	m.SetMetric("gross_liquidation_notional", data.NewMetric[float64](
		"gross_liquidation_notional",
		data.UnitNotional,
		data.TimescaleRollingWindow,
		0.0,
		tradeScale,
	).Write(grossLiq))
	m.SetMetric("net_liquidation_notional", data.NewMetric[float64](
		"net_liquidation_notional",
		data.UnitNotional,
		data.TimescaleRollingWindow,
		0.0,
		tradeScale,
	).Write(netLiq))
	m.SetMetric("gross_derivative_trade_notional", data.NewMetric[float64](
		"gross_derivative_trade_notional",
		data.UnitNotional,
		data.TimescaleRollingWindow,
		0.0,
		tradeScale,
	).Write(state.grossTradeTotal))

	var currentShare float64

	if grossLiq > 0 {
		m.SetMetric("liquidation_signed_fraction", data.NewMetric[float64](
			"liquidation_signed_fraction",
			data.UnitRatio,
			data.TimescaleRollingWindow,
			0.0,
			1.0,
		).Write(netLiq/grossLiq))
	}

	if state.grossTradeTotal > 0 {
		currentShare = grossLiq / state.grossTradeTotal
		m.SetMetric("liquidation_share", data.NewMetric[float64](
			"liquidation_share",
			data.UnitRatio,
			data.TimescaleRollingWindow,
			0.0,
			1.0,
		).Write(currentShare))
	}

	if advanced {
		duration := state.lastAdvancedTime.Sub(state.startTime).Seconds()

		if duration > 0 {
			rate := grossLiq / duration
			tradeRate := state.grossTradeTotal / duration
			m.SetMetric("liquidation_notional_rate", data.NewMetric[float64](
				"liquidation_notional_rate",
				data.UnitNotionalRate,
				data.TimescalePerSecond,
				0.0,
				tradeRate,
			).Write(rate))
		}

		if state.hasPrevLiqShare {
			shareVel := currentShare - state.prevLiqShare
			m.SetMetric("liquidation_share_velocity", data.NewMetric[float64](
				"liquidation_share_velocity",
				data.UnitVelocity,
				data.TimescalePerSecond,
				0.0,
				1.0,
			).Write(shareVel))
		}

		state.prevLiqShare = currentShare
		state.hasPrevLiqShare = true
	}

	m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(state.tradeCount, 'f', -1, 64))
}
