package liquidity

import (
	"context"
	"math"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
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
	"best_bid_price",
	"best_ask_price",
	"touch_quantity:bid",
	"touch_quantity:ask",
	"touch_notional:bid",
	"touch_notional:ask",
	"midpoint",
	"spread",
	"relative_spread",
	"two_sided_touch_notional",
	"touch_notional_imbalance",
	"touch_notional_baseline:bid",
	"touch_notional_baseline:ask",
	"relative_spread_baseline",
	"depth_ratio:bid",
	"depth_ratio:ask",
	"spread_ratio",
	"depth_divergence:bid",
	"depth_divergence:ask",
	"spread_divergence",
	"depth_noise_scale:bid",
	"depth_noise_scale:ask",
	"spread_noise_scale",
	"depth_zscore:bid",
	"depth_zscore:ask",
	"spread_zscore",
	"divergence_velocity:bid",
	"divergence_velocity:ask",
	"spread_divergence_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	books     broker.BookSource
	pipelines map[string]*nomagique.Number
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:     books,
		pipelines: make(map[string]*nomagique.Number),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", err))
	}

	return signal
}

func (signal *Signal) buildPipeline() *nomagique.Number {
	return nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					transport.NewSpread[float64](),
					// Stage 1: Pass through 14 raw values + Baselines for bid notional (4), ask notional (5), relative spread (8)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 14),
						nomagique.NewNumber(
							data.NewSelect(4, 5, 8),
							transport.NewParallel(
								adaptive.NewBaseline(adaptive.NewWindow()),
								adaptive.NewBaseline(adaptive.NewWindow()),
								adaptive.NewBaseline(adaptive.NewWindow()),
							),
						),
					),
					// Yields 14 + 6 = 20 values
					// Indices:
					// 0..13: raws (12=zero, 13=zero)
					// 14: bidCenter, 15: bidScale
					// 16: askCenter, 17: askScale
					// 18: spreadCenter, 19: spreadScale

					// Stage 2: Ratios and Divergences
					data.NewValue[core.Primitive](
						data.NewSlice(0, 20),
						nomagique.NewNumber(
							data.NewSelect(
								4, 14, // depth_ratio:bid = touchNotionalBid / bidCenter
								5, 16, // depth_ratio:ask = touchNotionalAsk / askCenter
								8, 18, // spread_ratio = relativeSpread / spreadCenter
								4, 14, // depth_divergence:bid = touchNotionalBid - bidCenter
								5, 16, // depth_divergence:ask = touchNotionalAsk - askCenter
								8, 18, // spread_divergence = relativeSpread - spreadCenter
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
							),
						),
					),
					// Yields 20 + 6 = 26 values
					// Indices:
					// 20: depth_ratio:bid
					// 21: depth_ratio:ask
					// 22: spread_ratio
					// 23: depth_divergence:bid
					// 24: depth_divergence:ask
					// 25: spread_divergence

					// Stage 3: Z-scores
					data.NewValue[core.Primitive](
						data.NewSlice(0, 26),
						nomagique.NewNumber(
							data.NewSelect(
								23, 15, // depth_zscore:bid = depth_divergence:bid / bidScale
								24, 17, // depth_zscore:ask = depth_divergence:ask / askScale
								25, 19, // spread_zscore = spread_divergence / spreadScale
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
					),
					// Yields 26 + 3 = 29 values
					// Indices:
					// 26: depth_zscore:bid
					// 27: depth_zscore:ask
					// 28: spread_zscore

					// Stage 4: Velocities
					data.NewValue[core.Primitive](
						data.NewSlice(0, 29),
						nomagique.NewNumber(
							data.NewSelect(
								23, 11, // divergence_velocity:bid (divergence, atNano)
								24, 11, // divergence_velocity:ask (divergence, atNano)
								25, 11, // spread_divergence_velocity (divergence, atNano)
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
							),
						),
					),
					// Yields 29 + 3 = 32 values
					// Indices:
					// 29: divergence_velocity:bid
					// 30: divergence_velocity:ask
					// 31: spread_divergence_velocity

					// Stage 5: Select exact outputKeys (31 values)
					data.NewSelect(
						0,  // best_bid_price
						1,  // best_ask_price
						2,  // touch_quantity:bid
						3,  // touch_quantity:ask
						4,  // touch_notional:bid
						5,  // touch_notional:ask
						6,  // midpoint
						7,  // spread
						8,  // relative_spread
						9,  // two_sided_touch_notional
						10, // touch_notional_imbalance
						14, // touch_notional_baseline:bid
						16, // touch_notional_baseline:ask
						18, // relative_spread_baseline
						20, // depth_ratio:bid
						21, // depth_ratio:ask
						22, // spread_ratio
						23, // depth_divergence:bid
						24, // depth_divergence:ask
						25, // spread_divergence
						15, // depth_noise_scale:bid
						17, // depth_noise_scale:ask
						19, // spread_noise_scale
						26, // depth_zscore:bid
						27, // depth_zscore:ask
						28, // spread_zscore
						29, // divergence_velocity:bid
						30, // divergence_velocity:ask
						31, // spread_divergence_velocity
						12, // historical_path_distance (0)
						13, // historical_path_percentile (0)
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "liquidity_state", data.NewValue[core.Primitive]()),
			),
		)
}

func (signal *Signal) pipelineFor(symbol string) *nomagique.Number {
	if pipe, ok := signal.pipelines[symbol]; ok {
		return pipe
	}

	pipe := signal.buildPipeline()
	signal.pipelines[symbol] = pipe
	return pipe
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", nil))
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		b := book.BestBid()
		a := book.BestAsk()

		if b == nil || a == nil || b.Price == nil || a.Price == nil ||
			b.Quantity == nil || a.Quantity == nil {
			return
		}

		bid = kraken.Float64(b.Price)
		bidQty = kraken.Float64(b.Quantity)
		ask = kraken.Float64(a.Price)
		askQty = kraken.Float64(a.Quantity)
		found = true
	})

	if !found {
		return nil
	}

	touch := map[string]float64{
		"bid":     bid,
		"ask":     ask,
		"bid_qty": bidQty,
		"ask_qty": askQty,
	}

	for _, value := range touch {
		if !broker.ValidTouchValue(value) {
			signal.Error(broker.InvalidTouch("liquidity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		signal.Error(broker.CrossedTouch("liquidity", prior.Label, bid, ask))
		return nil
	}

	if bidQty <= 0 || askQty <= 0 {
		return nil
	}

	touchNotionalBid := bid * bidQty
	touchNotionalAsk := ask * askQty
	midpoint := (bid + ask) / 2.0
	spread := ask - bid
	relativeSpread := spread / midpoint
	twoSidedNotional := math.Min(touchNotionalBid, touchNotionalAsk)
	total := touchNotionalBid + touchNotionalAsk
	imbalance := (touchNotionalBid - touchNotionalAsk) / total
	atNano := float64(prior.At.UnixNano())

	raws := []float64{
		bid,
		ask,
		bidQty,
		askQty,
		touchNotionalBid,
		touchNotionalAsk,
		midpoint,
		spread,
		relativeSpread,
		twoSidedNotional,
		imbalance,
		atNano,
		0.0,
		0.0,
	}

	output := make(map[string]float64)
	index := 0
	pipeline := signal.pipelineFor(prior.Label)

	for ptr := range pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&raws),
			),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.liquidity] overflow",
				nil,
			))
			return nil
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	return prior.Next(signal.Name(), output)
}
