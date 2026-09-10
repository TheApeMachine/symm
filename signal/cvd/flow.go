package cvd

import (
	"iter"
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
FlowInput is one aggressor execution and the quote facts that may accompany it.
*/
type FlowInput struct {
	Price, Quantity, Midpoint, PriorMid float64
	Buy, Quoted                         bool
	At, From                            int64
}

/*
Flow owns the cumulative execution arithmetic for one symbol.
*/
type Flow struct {
	core.Base[FlowInput, data.ProjectionInput]
	buyQty, sellQty, buyNotional, sellNotional float64
	buyCount, sellCount                        float64
	fraction                                   *adaptive.Baseline
	velocity                                   *temporal.Velocity
}

func newFlowGraph() *Flow {
	return &Flow{
		fraction: adaptive.NewBaseline(adaptive.NewWindow()),
		velocity: temporal.NewVelocity(),
	}
}

func (op *Flow) Next(
	in iter.Seq[core.Primitive[FlowInput, FlowInput]],
) iter.Seq[core.Primitive[data.ProjectionInput, data.ProjectionInput]] {
	return func(yield func(core.Primitive[data.ProjectionInput, data.ProjectionInput]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.observe(arriving.Read()))) {
				return
			}
		}
	}
}

func (op *Flow) observe(input FlowInput) data.ProjectionInput {
	notional := input.Price * input.Quantity
	values := map[string]float64{}
	flags := map[string]bool{}

	if input.Buy {
		op.buyQty += input.Quantity
		op.buyNotional += notional
		op.buyCount++
	}

	if !input.Buy {
		op.sellQty += input.Quantity
		op.sellNotional += notional
		op.sellCount++
	}

	tradeCount := op.buyCount + op.sellCount
	gross := op.buyNotional + op.sellNotional
	net := op.buyNotional - op.sellNotional
	grossQty := op.buyQty + op.sellQty
	netQty := op.buyQty - op.sellQty
	elapsed := float64(input.At-input.From) / float64(time.Second)
	signedCount := (op.buyCount - op.sellCount) / tradeCount
	signedNet := net / gross
	reading := op.fraction.Observe(signedNet)

	values["trade_count"] = tradeCount
	values["trade_count:buy"] = op.buyCount
	values["trade_count:sell"] = op.sellCount
	values["executed_quantity:buy"] = op.buyQty
	values["executed_quantity:sell"] = op.sellQty
	values["gross_executed_quantity"] = grossQty
	values["net_executed_quantity"] = netQty
	values["cumulative_volume_delta"] = netQty
	values["aggressive_notional:buy"] = op.buyNotional
	values["aggressive_notional:sell"] = op.sellNotional
	values["gross_notional"] = gross
	values["net_notional"] = net
	values["mean_trade_notional"] = gross / tradeCount
	values["cumulative_notional_delta"] = net
	values["signed_count_fraction"] = signedCount
	values["signed_net_fraction"] = signedNet
	values["cvd_epoch_from"] = float64(input.From) / float64(time.Second)
	values["signed_net_fraction_baseline"] = reading.Baseline
	values["signed_net_fraction_divergence"] = reading.Residual
	values["signed_net_fraction_zscore"] = reading.ZScore
	values["fraction_count"] = reading.Count
	values["fraction_residual"] = reading.Residual
	values["fraction_variance"] = reading.Variance
	flags["fraction_has_prior"] = reading.HasPrior
	flags["fraction_variance_defined"] = reading.VarianceDefined
	flags["rate_defined"] = elapsed > 0
	flags["quote_defined"] = input.Quoted

	if elapsed > 0 {
		values["trade_rate"] = tradeCount / elapsed
		values["gross_notional_rate"] = gross / elapsed
		values["net_notional_rate"] = net / elapsed
		values["buy_notional_rate"] = op.buyNotional / elapsed
		values["sell_notional_rate"] = op.sellNotional / elapsed
		velocity := op.velocity.Observe(net/elapsed, input.At)
		values["net_notional_rate_velocity"] = velocity.Rate
		flags["net_velocity_defined"] = velocity.Defined
	}

	if input.Quoted {
		logReturn := math.Log(input.Midpoint / input.PriorMid)
		values["midpoint_log_return"] = logReturn
		values["flow_aligned_midpoint_return"] = logReturn * math.Copysign(1, net)
		flags["response_defined"] = math.Abs(net) > 0

		if math.Abs(net) > 0 {
			values["midpoint_response_per_net_notional"] = logReturn / net
		}
	}

	return data.ProjectionInput{Values: values, Flags: flags}
}

func flowProjection() *data.Projection {
	p := &data.Projection{Source: "cvd"}
	for _, name := range []string{"trade_count", "trade_count:buy", "trade_count:sell", "executed_quantity:buy", "executed_quantity:sell", "gross_executed_quantity", "net_executed_quantity", "cumulative_volume_delta"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"aggressive_notional:buy", "aggressive_notional:sell", "gross_notional", "net_notional", "mean_trade_notional", "cumulative_notional_delta"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"signed_count_fraction", "signed_net_fraction"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"trade_rate", "gross_notional_rate", "net_notional_rate", "buy_notional_rate", "sell_notional_rate"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Defined: []string{"rate_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond})
	}
	p.Metrics = append(p.Metrics, data.MetricProjection{Label: "cvd_epoch_from", Path: []string{"cvd_epoch_from"}, Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous})
	for _, name := range []string{"signed_net_fraction_baseline", "signed_net_fraction_divergence", "signed_net_fraction_zscore"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Defined: []string{"fraction_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"midpoint_log_return", "flow_aligned_midpoint_return"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Defined: []string{"quote_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	p.Metrics = append(p.Metrics,
		data.MetricProjection{Label: "midpoint_response_per_net_notional", Path: []string{"midpoint_response_per_net_notional"}, Defined: []string{"response_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		data.MetricProjection{Label: "net_notional_rate_velocity", Path: []string{"net_notional_rate_velocity"}, Defined: []string{"net_velocity_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond})
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"fraction_count"}},
		{Name: data.MetadataDivergence, Path: []string{"fraction_residual"}, Defined: []string{"fraction_has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"fraction_variance"}, Defined: []string{"fraction_variance_defined"}},
	}
	return p
}
