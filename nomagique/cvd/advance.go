package cvd

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
flowState is one symbol's cumulative execution accounting and its causal
timeline head.
*/
type flowState struct {
	buyQty, sellQty, buyNotional, sellNotional float64
	buyCount, sellCount                        float64
	from, at                                   time.Time
	baseline                                   core.Primitive
	velocity                                   core.Primitive
}

/*
Flow owns the cumulative executed-flow arithmetic for every symbol. Each
arrival is accounted where it is computed: every metric is written into the
measurement at computation time, and undefined facts stay unwritten.
*/
type Flow struct {
	err    error
	states map[string]*flowState
}

func NewFlow() core.Primitive {
	return &Flow{states: make(map[string]*flowState)}
}

func (op *Flow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				state = &flowState{
					from:     m.At,
					baseline: adaptive.NewBaseline(adaptive.NewWindow()),
					velocity: temporal.NewVelocity(),
				}
				op.states[m.Label] = state
			}

			// A genuinely late event's facts are already accounted in the
			// cumulative totals it preceded; it advances nothing.
			if existing && m.At.Before(state.at) {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.observe(m, state)

			state.at = m.At

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Flow) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
observe accounts one execution into the symbol's cumulative state and writes
every defined flow metric into the measurement where it is computed.
*/
func (op *Flow) observe(m *data.Measurement[float64], state *flowState) {
	price := m.Metrics["price"].Raw
	quantity := m.Metrics["qty"].Raw
	notional := price * quantity
	buy := m.Provenance["side"] == "buy"

	if buy {
		state.buyQty += quantity
		state.buyNotional += notional
		state.buyCount++
	}

	if !buy {
		state.sellQty += quantity
		state.sellNotional += notional
		state.sellCount++
	}

	tradeCount := state.buyCount + state.sellCount
	gross := state.buyNotional + state.sellNotional
	net := state.buyNotional - state.sellNotional
	grossQty := state.buyQty + state.sellQty
	netQty := state.buyQty - state.sellQty
	elapsed := float64(m.At.Sub(state.from)) / float64(time.Second)
	signedCount := (state.buyCount - state.sellCount) / tradeCount
	signedNet := net / gross
	reading := drive[float64, adaptive.BaselineReading](state.baseline, &signedNet)

	m.Metrics["trade_count"] = m.Metrics["trade_count"].Write(tradeCount)
	m.Metrics["trade_count:buy"] = m.Metrics["trade_count:buy"].Write(state.buyCount)
	m.Metrics["trade_count:sell"] = m.Metrics["trade_count:sell"].Write(state.sellCount)
	m.Metrics["executed_quantity:buy"] = m.Metrics["executed_quantity:buy"].Write(state.buyQty)
	m.Metrics["executed_quantity:sell"] = m.Metrics["executed_quantity:sell"].Write(state.sellQty)
	m.Metrics["gross_executed_quantity"] = m.Metrics["gross_executed_quantity"].Write(grossQty)
	m.Metrics["net_executed_quantity"] = m.Metrics["net_executed_quantity"].Write(netQty)
	m.Metrics["cumulative_volume_delta"] = m.Metrics["cumulative_volume_delta"].Write(netQty)
	m.Metrics["aggressive_notional:buy"] = m.Metrics["aggressive_notional:buy"].Write(state.buyNotional)
	m.Metrics["aggressive_notional:sell"] = m.Metrics["aggressive_notional:sell"].Write(state.sellNotional)
	m.Metrics["gross_notional"] = m.Metrics["gross_notional"].Write(gross)
	m.Metrics["net_notional"] = m.Metrics["net_notional"].Write(net)
	m.Metrics["mean_trade_notional"] = m.Metrics["mean_trade_notional"].Write(gross / tradeCount)
	m.Metrics["cumulative_notional_delta"] = m.Metrics["cumulative_notional_delta"].Write(net)
	m.Metrics["signed_count_fraction"] = m.Metrics["signed_count_fraction"].Write(signedCount)
	m.Metrics["signed_net_fraction"] = m.Metrics["signed_net_fraction"].Write(signedNet)
	m.Metrics["cvd_epoch_from"] = m.Metrics["cvd_epoch_from"].Write(float64(state.from.UnixNano()) / float64(time.Second))

	m.From = state.from

	m.Metadata[data.MetadataSupport] = reading.Count

	if reading.HasPrior {
		m.Metrics["signed_net_fraction_baseline"] = m.Metrics["signed_net_fraction_baseline"].Write(reading.Baseline)
		m.Metrics["signed_net_fraction_divergence"] = m.Metrics["signed_net_fraction_divergence"].Write(reading.Residual)
		m.Metrics["signed_net_fraction_zscore"] = m.Metrics["signed_net_fraction_zscore"].Write(reading.ZScore)
		m.Metadata[data.MetadataDivergence] = reading.Residual

		if reading.VarianceDefined {
			m.Metadata[data.MetadataNoiseVariance] = reading.Variance
		}
	}

	if elapsed > 0 {
		m.Metrics["trade_rate"] = m.Metrics["trade_rate"].Write(tradeCount / elapsed)
		m.Metrics["gross_notional_rate"] = m.Metrics["gross_notional_rate"].Write(gross / elapsed)
		m.Metrics["net_notional_rate"] = m.Metrics["net_notional_rate"].Write(net / elapsed)
		m.Metrics["buy_notional_rate"] = m.Metrics["buy_notional_rate"].Write(state.buyNotional / elapsed)
		m.Metrics["sell_notional_rate"] = m.Metrics["sell_notional_rate"].Write(state.sellNotional / elapsed)

		observation := temporal.Observation{Value: net / elapsed, At: m.At.UnixNano()}
		velocity := drive[temporal.Observation, temporal.VelocityReading](state.velocity, &observation)

		if velocity.Defined {
			m.Metrics["net_notional_rate_velocity"] = m.Metrics["net_notional_rate_velocity"].Write(velocity.Rate)
		}
	}
}
