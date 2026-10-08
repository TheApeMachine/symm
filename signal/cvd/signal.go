package cvd

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
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
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewValue[core.Primitive](
						nomagique.NewNumber(
							data.NewSlice(0, 3),
							transport.NewParallel(
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),
										arithmetic.NewAdd(),
										nomagique.NewNumber(
											data.NewValue(
												arithmetic.NewSubtract(),
												arithmetic.NewAdd(),
											),
											arithmetic.NewDivide(),
										),
									),
								),
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue(
										transport.NewPass(),
										arithmetic.NewAdd(),
										arithmetic.NewSubtract(),
										arithmetic.NewSubtract(),
									),
								),
								nomagique.NewNumber(
									vector.NewScale(),
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),
										arithmetic.NewAdd(),
										arithmetic.NewSubtract(),
										arithmetic.NewSubtract(),
										nomagique.NewNumber(
											data.NewValue(
												arithmetic.NewSubtract(),
												arithmetic.NewAdd(),
											),
											arithmetic.NewDivide(),
										),
									),
								),
							),
						),
						data.NewSlice(3, 8),
					),
					data.NewValue[core.Primitive](
						data.NewSlice(0, 19),
						nomagique.NewNumber(
							data.NewSelect(
								11, 2,
								2, 14,
								11, 14,
								12, 14,
								9, 14,
								10, 14,
								17, 14,
								18, 12,
								0, 0,
								1, 1,
								2, 2,
								3, 3,
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
							),
						),
					),
					data.NewValue[core.Primitive](
						data.NewSlice(0, 31),
						nomagique.NewNumber(
							data.NewSelect(
								21, 21,
								13, 13,
								25, 25,
								21, 21,
								22, 22,
								0, 0,
								1, 1,
								2, 2,
								3, 3,
								4, 4,
								5, 5,
								6, 6,
								7, 7,
								8, 8,
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
							),
						),
					),
					data.NewValue[core.Primitive](
						data.NewSlice(0, 45),
						nomagique.NewNumber(
							data.NewSelect(
								21, 31,
								21, 31,
								21, 31, 32,
								13, 33,
								13, 33, 34,
								25, 35,
								25, 35, 36,
							),
							data.NewBatch(4, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 2), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 2), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), data.NewValue(arithmetic.NewSubtract(), transport.NewPass()), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 2), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), data.NewValue(arithmetic.NewSubtract(), transport.NewPass()), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 2), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), data.NewValue(arithmetic.NewSubtract(), transport.NewPass()), arithmetic.NewDivide()),
							),
						),
					),
					data.NewSelect(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 33, 34, 35, 22, 36, 37, 38, 48, 49, 50, 40, 51, 52, 42, 53, 54, 44, 45, 46, 47),
				),
				data.NewMessage(data.WRITE, "symbolstore", "cvd_state", data.NewValue[core.Primitive]()),
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

	counts := []float64{buyCount, sellCount}
	quantities := []float64{buyQty, sellQty}
	notionals := [2][]float64{quantities, {price}}

	timeDelta := float64(prior.At.Sub(prior.From).Seconds())

	if timeDelta == 0 {
		timeDelta = 1
	}

	epochFrom := float64(prior.From.UnixNano())
	midpointFrom := 0.0
	midpointAt := 0.0
	midpointLogReturn := 0.0

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				counts,
				quantities,
				notionals,
				timeDelta,
				epochFrom,
				midpointFrom,
				midpointAt,
				midpointLogReturn,
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

	return prior.Next(signal.Name(), output)
}
