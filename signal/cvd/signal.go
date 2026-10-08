package cvd

import (
	"context"
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

var outputKeys = []string{
	"trade_count:buy",
	"trade_count:sell",
	"trade_count",
	"signed_count_fraction",
	"executed_quantity:buy",
	"executed_quantity:sell",
	"gross_executed_quantity",
	"net_executed_quantity",
	"cumulative_volume_delta",
	"aggressive_notional:buy",
	"aggressive_notional:sell",
	"gross_notional",
	"net_notional",
	"cumulative_notional_delta",
	"signed_net_fraction",
	"mean_trade_notional",
	"trade_rate",
	"gross_notional_rate",
	"net_notional_rate",
	"buy_notional_rate",
	"sell_notional_rate",
	"cvd_epoch_from",
	"response_midpoint:from",
	"response_midpoint:at",
	"midpoint_log_return",
	"midpoint_return_rate",
	"flow_aligned_midpoint_return",
	"midpoint_response_per_net_notional",
	"gross_notional_rate_baseline",
	"gross_notional_rate_ratio",
	"gross_notional_rate_divergence",
	"gross_notional_rate_zscore",
	"signed_net_fraction_baseline",
	"signed_net_fraction_divergence",
	"signed_net_fraction_zscore",
	"midpoint_return_rate_baseline",
	"midpoint_return_rate_divergence",
	"midpoint_return_rate_zscore",
	"net_notional_rate_velocity",
	"gross_notional_rate_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(func() core.Primitive {
					return nomagique.NewNumber(
						transport.NewParallel(
							// 0: trade_count:buy
							vector.NewScale(1.0),
							// 1: trade_count:sell
							vector.NewScale(1.0),
							// 2: trade_count
							arithmetic.NewAdd(),
							// 3: signed_count_fraction
							nomagique.NewNumber(
								data.NewBatch(2, 2),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
							),
							// 4: executed_quantity:buy
							vector.NewScale(1.0),
							// 5: executed_quantity:sell
							vector.NewScale(1.0),
							// 6: gross_executed_quantity
							arithmetic.NewAdd(),
							// 7: net_executed_quantity
							arithmetic.NewSubtract(),
							// 8: cumulative_volume_delta
							nomagique.NewNumber(arithmetic.NewSubtract(), statistic.NewSum()),
							// 9: aggressive_notional:buy
							vector.NewScale(),
							// 10: aggressive_notional:sell
							vector.NewScale(),
							// 11: gross_notional
							nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
							// 12: net_notional
							nomagique.NewNumber(vector.NewScale(), arithmetic.NewSubtract()),
							// 13: cumulative_notional_delta
							nomagique.NewNumber(nomagique.NewNumber(vector.NewScale(), arithmetic.NewSubtract()), statistic.NewSum()),
							// 14: signed_net_fraction
							nomagique.NewNumber(
								vector.NewScale(),
								data.NewBatch(2, 2),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
							),
							// 15: mean_trade_notional
							nomagique.NewNumber(
								data.NewBatch(4, 2),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
							),
							// 16: trade_rate
							nomagique.NewNumber(
								data.NewBatch(2, 1),
								transport.NewParallel(
									arithmetic.NewAdd(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 17: gross_notional_rate
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 18: net_notional_rate
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewSubtract()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 19: buy_notional_rate
							nomagique.NewNumber(
								data.NewBatch(2, 1),
								transport.NewParallel(
									vector.NewScale(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 20: sell_notional_rate
							nomagique.NewNumber(
								data.NewBatch(2, 1),
								transport.NewParallel(
									vector.NewScale(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 21: cvd_epoch_from
							store.NewConstant[float64](),
							// 22: response_midpoint:from
							vector.NewScale(1.0),
							// 23: response_midpoint:at
							vector.NewScale(1.0),
							// 24: midpoint_log_return
							vector.NewScale(1.0),
							// 25: midpoint_return_rate
							arithmetic.NewDivide(),
							// 26: flow_aligned_midpoint_return
							vector.NewScale(1.0),
							// 27: midpoint_response_per_net_notional
							nomagique.NewNumber(
								data.NewBatch(1, 4),
								transport.NewParallel(
									vector.NewScale(1.0),
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewSubtract()),
								),
								arithmetic.NewDivide(),
							),
							// 28: gross_notional_rate_baseline
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
								adaptive.NewBaseline(adaptive.NewWindow()),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									transport.NewDiscard(),
								),
							),
							// 29: gross_notional_rate_ratio
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									nomagique.NewNumber(
										adaptive.NewBaseline(adaptive.NewWindow()),
										data.NewBatch(1, 1),
										transport.NewParallel(
											vector.NewScale(1.0),
											transport.NewDiscard(),
										),
									),
								),
								arithmetic.NewDivide(),
							),
							// 30: gross_notional_rate_divergence
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									nomagique.NewNumber(
										adaptive.NewBaseline(adaptive.NewWindow()),
										data.NewBatch(1, 1),
										transport.NewParallel(
											vector.NewScale(1.0),
											transport.NewDiscard(),
										),
									),
								),
								arithmetic.NewSubtract(),
							),
							// 31: gross_notional_rate_zscore
							nomagique.NewNumber(
								data.NewBatch(4, 1),
								transport.NewParallel(
									nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									adaptive.NewBaseline(adaptive.NewWindow()),
								),
								data.NewBatch(2, 1),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 32: signed_net_fraction_baseline
							nomagique.NewNumber(
								vector.NewScale(),
								data.NewBatch(2, 2),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
								adaptive.NewBaseline(adaptive.NewWindow()),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									transport.NewDiscard(),
								),
							),
							// 33: signed_net_fraction_divergence
							nomagique.NewNumber(
								vector.NewScale(),
								data.NewBatch(2, 2),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									nomagique.NewNumber(
										adaptive.NewBaseline(adaptive.NewWindow()),
										data.NewBatch(1, 1),
										transport.NewParallel(
											vector.NewScale(1.0),
											transport.NewDiscard(),
										),
									),
								),
								arithmetic.NewSubtract(),
							),
							// 34: signed_net_fraction_zscore
							nomagique.NewNumber(
								vector.NewScale(),
								data.NewBatch(2, 2),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									arithmetic.NewAdd(),
								),
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									adaptive.NewBaseline(adaptive.NewWindow()),
								),
								data.NewBatch(2, 1),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 35: midpoint_return_rate_baseline
							nomagique.NewNumber(
								arithmetic.NewDivide(),
								adaptive.NewBaseline(adaptive.NewWindow()),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									transport.NewDiscard(),
								),
							),
							// 36: midpoint_return_rate_divergence
							nomagique.NewNumber(
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									nomagique.NewNumber(
										adaptive.NewBaseline(adaptive.NewWindow()),
										data.NewBatch(1, 1),
										transport.NewParallel(
											vector.NewScale(1.0),
											transport.NewDiscard(),
										),
									),
								),
								arithmetic.NewSubtract(),
							),
							// 37: midpoint_return_rate_zscore
							nomagique.NewNumber(
								arithmetic.NewDivide(),
								data.NewRepeat(2),
								data.NewBatch(1, 1),
								transport.NewParallel(
									vector.NewScale(1.0),
									adaptive.NewBaseline(adaptive.NewWindow()),
								),
								data.NewBatch(2, 1),
								transport.NewParallel(
									arithmetic.NewSubtract(),
									vector.NewScale(1.0),
								),
								arithmetic.NewDivide(),
							),
							// 38: net_notional_rate_velocity
							nomagique.NewNumber(
								data.NewBatch(5, 1),
								transport.NewParallel(
									nomagique.NewNumber(
										data.NewBatch(4, 1),
										transport.NewParallel(
											nomagique.NewNumber(vector.NewScale(), arithmetic.NewSubtract()),
											vector.NewScale(1.0),
										),
										arithmetic.NewDivide(),
									),
									vector.NewScale(1.0),
								),
								temporal.NewVelocity(),
							),
							// 39: gross_notional_rate_velocity
							nomagique.NewNumber(
								data.NewBatch(5, 1),
								transport.NewParallel(
									nomagique.NewNumber(
										data.NewBatch(4, 1),
										transport.NewParallel(
											nomagique.NewNumber(vector.NewScale(), arithmetic.NewAdd()),
											vector.NewScale(1.0),
										),
										arithmetic.NewDivide(),
									),
									vector.NewScale(1.0),
								),
								temporal.NewVelocity(),
							),
							// 40: historical_path_distance
							vector.NewScale(1.0),
							// 41: historical_path_percentile
							vector.NewScale(1.0),
						),
					)
				}),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	side := prior.Meta("side")

	if side != "buy" && side != "sell" {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[signal.cvd] no side",
			nil,
		))

		return nil
	}

	price := data.Pull(prior.Read("price")).Metric.Raw
	qty := data.Pull(prior.Read("qty")).Metric.Raw

	var (
		buyQty, sellQty     float64
		buyCount, sellCount float64
	)

	if side == "buy" {
		buyCount = 1
		buyQty = qty
	}

	if side == "sell" {
		sellCount = 1
		sellQty = qty
	}

	timeDelta := float64(prior.At.Sub(prior.From).Seconds())

	if timeDelta == 0 {
		timeDelta = 1
	}

	epochFrom := float64(prior.At.UnixNano())
	midpointFrom := 0.0
	midpointAt := 0.0
	midpointLogReturn := 0.0
	atNanos := float64(prior.At.UnixNano())

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.EVALUATE,
			"symbolstore",
			prior.Label+"/epoch:"+strconv.FormatInt(prior.Epoch, 10),
			data.NewValue[core.Primitive](
				// 0: trade_count:buy
				data.NewValue(buyCount),
				// 1: trade_count:sell
				data.NewValue(sellCount),
				// 2: trade_count
				data.NewValue(buyCount, sellCount),
				// 3: signed_count_fraction
				data.NewValue(buyCount, sellCount, buyCount, sellCount),
				// 4: executed_quantity:buy
				data.NewValue(buyQty),
				// 5: executed_quantity:sell
				data.NewValue(sellQty),
				// 6: gross_executed_quantity
				data.NewValue(buyQty, sellQty),
				// 7: net_executed_quantity
				data.NewValue(buyQty, sellQty),
				// 8: cumulative_volume_delta
				data.NewValue(buyQty, sellQty),
				// 9: aggressive_notional:buy
				data.NewValue(buyQty, price),
				// 10: aggressive_notional:sell
				data.NewValue(sellQty, price),
				// 11: gross_notional
				data.NewValue(buyQty, price, sellQty, price),
				// 12: net_notional
				data.NewValue(buyQty, price, sellQty, price),
				// 13: cumulative_notional_delta
				data.NewValue(buyQty, price, sellQty, price),
				// 14: signed_net_fraction
				data.NewValue(buyQty, price, sellQty, price, buyQty, price, sellQty, price),
				// 15: mean_trade_notional
				data.NewValue(buyQty, price, sellQty, price, buyCount, sellCount),
				// 16: trade_rate
				data.NewValue(buyCount, sellCount, timeDelta),
				// 17: gross_notional_rate
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 18: net_notional_rate
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 19: buy_notional_rate
				data.NewValue(buyQty, price, timeDelta),
				// 20: sell_notional_rate
				data.NewValue(sellQty, price, timeDelta),
				// 21: cvd_epoch_from
				data.NewValue(epochFrom),
				// 22: response_midpoint:from
				data.NewValue(midpointFrom),
				// 23: response_midpoint:at
				data.NewValue(midpointAt),
				// 24: midpoint_log_return
				data.NewValue(midpointLogReturn),
				// 25: midpoint_return_rate
				data.NewValue(midpointLogReturn, timeDelta),
				// 26: flow_aligned_midpoint_return
				data.NewValue(midpointLogReturn),
				// 27: midpoint_response_per_net_notional
				data.NewValue(midpointLogReturn, buyQty, price, sellQty, price),
				// 28: gross_notional_rate_baseline
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 29: gross_notional_rate_ratio
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 30: gross_notional_rate_divergence
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 31: gross_notional_rate_zscore
				data.NewValue(buyQty, price, sellQty, price, timeDelta),
				// 32: signed_net_fraction_baseline
				data.NewValue(buyQty, price, sellQty, price, buyQty, price, sellQty, price),
				// 33: signed_net_fraction_divergence
				data.NewValue(buyQty, price, sellQty, price, buyQty, price, sellQty, price),
				// 34: signed_net_fraction_zscore
				data.NewValue(buyQty, price, sellQty, price, buyQty, price, sellQty, price),
				// 35: midpoint_return_rate_baseline
				data.NewValue(midpointLogReturn, timeDelta),
				// 36: midpoint_return_rate_divergence
				data.NewValue(midpointLogReturn, timeDelta),
				// 37: midpoint_return_rate_zscore
				data.NewValue(midpointLogReturn, timeDelta),
				// 38: net_notional_rate_velocity
				data.NewValue(buyQty, price, sellQty, price, timeDelta, atNanos),
				// 39: gross_notional_rate_velocity
				data.NewValue(buyQty, price, sellQty, price, timeDelta, atNanos),
				// 40: historical_path_distance
				data.NewValue(0.0),
				// 41: historical_path_percentile
				data.NewValue(0.0),
			),
		).Next(nil),
	) {
		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.cvd] overflow",
				nil,
			))

			return nil
		}

		if ptr == nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.cvd] pipeline returned nil",
				nil,
			))

			return nil
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	if err := signal.pipeline.Error(); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[signal.cvd] pipeline failed", err))
		return nil
	}

	if index != len(outputKeys) {
		signal.Error(errnie.Err(errnie.UnprocessableContent, "[signal.cvd] incomplete output", core.ErrShape))
		return nil
	}

	return prior.Next(signal.Name(), output)
}
