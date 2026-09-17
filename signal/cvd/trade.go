package cvd

import (
	"context"
	"iter"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
	cvd "github.com/theapemachine/symm/nomagique/statistic/cvd"
)

/*
Trade is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is composed nomagique pipelines per symbol
over the measurements — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipelines map[string]core.Primitive
	mu        sync.RWMutex
	ID        int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{
		pipelines: make(map[string]core.Primitive),
	}

	trade.System = runtime.NewSystem(ctx, "cvd:trade", trade)
	return trade
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	trade.mu.RLock()
	pipeline, ok := trade.pipelines[symbol]
	trade.mu.RUnlock()

	if ok {
		return pipeline
	}

	trade.mu.Lock()
	defer trade.mu.Unlock()

	pipeline, ok = trade.pipelines[symbol]
	if ok {
		return pipeline
	}

	pipeline = nomagique.NewNumber(cvd.
		NewGate(), cvd.
		NewQuantity(), cvd.
		NewNotional(), cvd.
		NewRates(), data.NewFinalizer[float64](),
	)
	trade.pipelines[symbol] = pipeline
	return pipeline
}

/*
Next supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
	inputs:
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if trade.Status() != runtime.READY {
				errnie.Warn(trade.Name() + ": Next called before READY; dropping event")
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if measurement == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if len(measurement.Peers) > 0 {
				peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
					_, hasPrice := candidate.Metrics["price"]
					_, hasQty := candidate.Metrics["qty"]
					return hasPrice && hasQty && candidate.Label != ""
				})

				if peer == nil {
					continue inputs
				}

				measurement.Pull(peer, "price", "qty")
			}

			if measurement.Label == "" {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if _, hasPrice := measurement.Metrics["price"]; !hasPrice {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if _, hasQty := measurement.Metrics["qty"]; !hasQty {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			res := sequence.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(sequence.NewOne(unsafe.Pointer(&measurement)).Next(nil)))

			if res == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if res != nil && !yield(unsafe.Pointer(&res)) {
				return
			}
			continue inputs

		}
	}
}

/*
Register returns the pre-allocated measurement every trade flows through:
every metric the instrument can produce is declared, none valued.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("cvd", map[string]data.Metric[float64]{
		"trade_count": data.NewMetric[float64](
			"trade_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_count:buy": data.NewMetric[float64](
			"trade_count:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_count:sell": data.NewMetric[float64](
			"trade_count:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"executed_quantity:buy": data.NewMetric[float64](
			"executed_quantity:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"executed_quantity:sell": data.NewMetric[float64](
			"executed_quantity:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_executed_quantity": data.NewMetric[float64](
			"gross_executed_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"net_executed_quantity": data.NewMetric[float64](
			"net_executed_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cumulative_volume_delta": data.NewMetric[float64](
			"cumulative_volume_delta", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"aggressive_notional:buy": data.NewMetric[float64](
			"aggressive_notional:buy", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"aggressive_notional:sell": data.NewMetric[float64](
			"aggressive_notional:sell", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_notional": data.NewMetric[float64](
			"gross_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional": data.NewMetric[float64](
			"net_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"mean_trade_notional": data.NewMetric[float64](
			"mean_trade_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"cumulative_notional_delta": data.NewMetric[float64](
			"cumulative_notional_delta", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_rate": data.NewMetric[float64](
			"trade_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_notional_rate": data.NewMetric[float64](
			"gross_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional_rate": data.NewMetric[float64](
			"net_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"buy_notional_rate": data.NewMetric[float64](
			"buy_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"sell_notional_rate": data.NewMetric[float64](
			"sell_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional_rate_velocity": data.NewMetric[float64](
			"net_notional_rate_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_count_fraction": data.NewMetric[float64](
			"signed_count_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction": data.NewMetric[float64](
			"signed_net_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cvd_epoch_from": data.NewMetric[float64](
			"cvd_epoch_from", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_baseline": data.NewMetric[float64](
			"signed_net_fraction_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_divergence": data.NewMetric[float64](
			"signed_net_fraction_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_zscore": data.NewMetric[float64](
			"signed_net_fraction_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
