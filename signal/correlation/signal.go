package correlation

import (
	"context"
	"unsafe"

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
)

var outputKeys = []string{
	"last_price",
	"observation_count",
	"signed_correlation",
	"absolute_correlation",
	"cohort_signed_correlation",
	"cohort_absolute_correlation",
	"covariance",
	"return_energy:reference",
	"return_energy:measured",
	"return_energy_rate:reference",
	"return_energy_rate:measured",
	"peer_return_energy_rate",
	"focal_return_energy_rate",
	"supported_return_count:measured",
	"supported_return_count:reference",
	"shared_time",
	"overlap_density",
	"overlap_pair_count",
	"effective_sample_count",
	"correlation_p_value",
	"correlation_standard_error_fisher",
	"cohort_peer_count",
	"cohort_correlation_dispersion",
	"cohort_effective_peer_count",
	"relative_return_energy",
	"relative_cohort_return_energy",
	"correlation_baseline",
	"correlation_divergence",
	"correlation_zscore",
	"correlation_velocity",
	"relative_return_energy_baseline",
	"relative_return_energy_divergence",
	"relative_return_energy_zscore",
	"relative_return_energy_velocity",
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
							data.NewSlice(0, 2),
							transport.NewParallel(
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),
										arithmetic.NewAdd(),
										arithmetic.NewSubtract(),
										nomagique.NewNumber(
											data.NewValue[core.Primitive](
												arithmetic.NewSubtract(),
												arithmetic.NewAdd(),
											),
											arithmetic.NewDivide(),
										),
									),
								),
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),
										arithmetic.NewAdd(),
										arithmetic.NewSubtract(),
									),
								),
							),
						),
						data.NewSlice(2, 4),
					),
					data.NewValue[core.Primitive](
						data.NewSlice(0, 11),
						nomagique.NewNumber(
							data.NewSelect(
								0, 9,
								1, 9,
								2, 9,
								3, 9,
								5, 9,
								6, 9,
								7, 9,
								8, 9,
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
						data.NewSlice(0, 23),
						nomagique.NewNumber(
							data.NewSelect(
								11, 11,
								12, 12,
								13, 13,
								14, 14,
								15, 15,
								16, 16,
								17, 17,
								18, 18,
								19, 19,
								20, 20,
								21, 21,
								22, 22,
								11, 11,
								12, 12,
								13, 13,
								14, 14,
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
					),
					data.NewSelect(
						0, 5, 2, 3, 4, 1, 6, 7, 8, 11,
						12, 13, 14, 15, 16, 9, 17, 18, 19, 20,
						21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
						31, 32, 33, 34,
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "correlation_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[signal.correlation] no price",
			nil,
		))
		return nil
	}
	price := priceEntry.Metric.Raw

	counts := []float64{1, 1}
	prices := []float64{price, price}

	timeDelta := float64(prior.At.Sub(prior.From).Seconds())
	if timeDelta == 0 {
		timeDelta = 1
	}
	epochFrom := float64(prior.From.UnixNano())

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&prices),
				unsafe.Pointer(&counts),
				unsafe.Pointer(&timeDelta),
				unsafe.Pointer(&epochFrom),
			),
		).Next(nil),
	) {
		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.correlation] overflow",
				nil,
			))

			return nil
		}

		if ptr == nil {
			continue
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	return prior.Next(signal.Name(), output)
}
