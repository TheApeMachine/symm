package liquidity

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/transport"
)

type GraphInput struct {
	BestBid, BestAsk, BidQty, AskQty float64
	At                               int64
}

type Graph struct {
	core.Base[GraphInput, data.ProjectionInput]
	estimator *joint.Estimator
	velocity  [3]*linear.LocalRegression
}

func newLiquidityGraph() *Graph {
	return &Graph{
		estimator: joint.NewEstimator(equation.NewWelford(), equation.NewWelford(), equation.NewWelford()),
		velocity:  [3]*linear.LocalRegression{linear.NewLocalRegression(), linear.NewLocalRegression(), linear.NewLocalRegression()},
	}
}

func (op *Graph) Next(
	in iter.Seq[core.Primitive[GraphInput, GraphInput]],
) iter.Seq[core.Primitive[data.ProjectionInput, data.ProjectionInput]] {
	return func(yield func(core.Primitive[data.ProjectionInput, data.ProjectionInput]) bool) {
		for arriving := range in {
			projected, err := op.observe(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(projected)) {
				return
			}
		}
	}
}

func (op *Graph) observe(input GraphInput) (data.ProjectionInput, error) {
	bidNotional := input.BestBid * input.BidQty
	askNotional := input.BestAsk * input.AskQty
	midpoint := (input.BestBid + input.BestAsk) / 2
	spread := input.BestAsk - input.BestBid
	relative := spread / midpoint
	values := map[string]float64{
		"best_bid_price":           input.BestBid,
		"best_ask_price":           input.BestAsk,
		"touch_quantity:bid":       input.BidQty,
		"touch_quantity:ask":       input.AskQty,
		"touch_notional:bid":       bidNotional,
		"touch_notional:ask":       askNotional,
		"midpoint":                 midpoint,
		"spread":                   spread,
		"relative_spread":          relative,
		"two_sided_touch_notional": math.Min(bidNotional, askNotional),
		"touch_notional_imbalance": (bidNotional - askNotional) / (bidNotional + askNotional),
	}
	flags := map[string]bool{}
	logged := []float64{math.Log(bidNotional), math.Log(askNotional), math.Log(relative)}
	originals := []float64{bidNotional, askNotional, relative}
	names := []string{"bid", "ask", "spread"}
	result, err := transport.Evaluate(op.estimator, transport.Values(joint.Input{Values: logged}))

	if err != nil {
		return data.ProjectionInput{}, err
	}

	values["snr"] = result.SNR
	flags["snr_defined"] = result.SNRDefined

	for index, name := range names {
		channel := result.Channels[index]
		flags[name+"_has_prior"] = channel.HasPrior
		flags[name+"_noise_defined"] = channel.ScoreScale > 0
		flags[name+"_slope_defined"] = false
		flags[name+"_snr_defined"] = false
		values[name+"_count"] = channel.Count
		values[name+"_residual"] = channel.Residual

		if channel.HasPrior {
			values[name+"_baseline"] = channel.Baseline
			values[name+"_ratio"] = originals[index] / channel.Baseline
			values[name+"_zscore"] = channel.ZScore
			values[name+"_noise"] = channel.ScoreScale
			summary, err := transport.Evaluate(op.velocity[index], transport.Values(equation.Price{At: input.At, Value: channel.Residual}))

			if err != nil {
				return data.ProjectionInput{}, err
			}

			flags[name+"_slope_defined"] = summary.SlopeDefined
			flags[name+"_snr_defined"] = summary.SNRDefined
			values[name+"_slope"] = summary.Slope
			values[name+"_velocity_snr"] = summary.SNR
		}
	}

	return data.ProjectionInput{Values: values, Flags: flags}, nil
}

func liquidityProjection() *data.Projection {
	p := &data.Projection{Source: "liquidity"}
	for _, name := range []string{"best_bid_price", "best_ask_price", "touch_notional:bid", "touch_notional:ask", "midpoint", "spread", "two_sided_touch_notional"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"touch_quantity:bid", "touch_quantity:ask"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"relative_spread", "touch_notional_imbalance"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, side := range []string{"bid", "ask", "spread"} {
		names := []string{"touch_notional_baseline:" + side, "depth_ratio:" + side, "depth_divergence:" + side, "depth_noise_scale:" + side, "depth_zscore:" + side, "divergence_velocity:" + side, "divergence_velocity_snr:" + side}
		keys := []string{side + "_baseline", side + "_ratio", side + "_residual", side + "_noise", side + "_zscore"}
		if side == "spread" {
			names = []string{"relative_spread_baseline", "spread_ratio", "spread_divergence", "spread_noise_scale", "spread_zscore", "spread_divergence_velocity", "spread_divergence_velocity_snr"}
		}
		for index, key := range keys {
			gate := side + "_has_prior"
			if index >= 3 {
				gate = side + "_noise_defined"
			}
			unit := data.UnitDimensionless
			if index == 0 && side != "spread" {
				unit = data.UnitRate
			}
			p.Metrics = append(p.Metrics, data.MetricProjection{Label: names[index], Path: []string{key}, Defined: []string{gate}, Unit: unit, Timescale: data.TimescaleInstantaneous})
		}
		p.Metrics = append(p.Metrics,
			data.MetricProjection{Label: names[5], Path: []string{side + "_slope"}, Defined: []string{side + "_slope_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.MetricProjection{Label: names[6], Path: []string{side + "_velocity_snr"}, Defined: []string{side + "_snr_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"bid_count"}},
		{Name: data.MetadataMahalanobisSNR, Path: []string{"snr"}, Defined: []string{"snr_defined"}},
	}
	return p
}
