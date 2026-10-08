package pumpdump

import (
	"context"
	"time"
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
	"trade_price",
	"trade_quantity",
	"trade_notional",
	"trade_interval_seconds",
	"volume_bar_target_quantity",
	"volume_bar_quantity",
	"volume_bar_notional",
	"volume_bar_trade_count",
	"volume_bar_duration",
	"volume_rate",
	"notional_rate",
	"trade_rate",
	"completed_bars",
	"notional_rate_baseline",
	"notional_rate_ratio",
	"notional_rate_divergence",
	"notional_rate_zscore",
	"notional_rate_velocity",
	"best_bid",
	"best_ask",
	"midpoint",
	"spread",
	"relative_spread",
	"relative_spread_baseline",
	"spread_ratio",
	"spread_divergence",
	"spread_zscore",
	"spread_divergence_velocity",
	"midpoint:from",
	"midpoint:at",
	"midpoint_log_return",
	"midpoint_return_rate",
	"positive_midpoint_return",
	"negative_midpoint_return",
	"midpoint_return_baseline",
	"midpoint_return_divergence",
	"midpoint_return_zscore",
	"midpoint_return_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	books    broker.BookSource
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					// Stage 1: VolumeBar & Touch routing
					// Input slices: [0: raws (price, qty, atNanos, midpoint), 1: touch (bid, ask, midpoint, spread, relativeSpread), 2: price, 3: qty]
					data.NewValue[core.Primitive](
						nomagique.NewNumber(
							data.NewSlice(0, 3),
							transport.NewParallel(
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									temporal.NewVolumeBar(1.0),
								),
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									transport.NewPass(),
								),
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									transport.NewPass(),
								),
							),
						),
						// Yields 18 (volumebar) + 5 (touch) + 2 (price, qty) = 25 values
					),
					// Stage 2: Baselines & Velocities
					data.NewValue[core.Primitive](
						data.NewSlice(0, 25),
						nomagique.NewNumber(
							data.NewSelect(
								8, 8, // notional_rate -> baseline
								22, 22, // relative_spread -> baseline
								13, 13, // midpoint_log_return -> baseline
								8, 8, // notional_rate -> velocity
								22, 22, // relative_spread -> velocity
								13, 13, // midpoint_log_return -> velocity
								0, 0, // historical_path_distance (0)
								0, 0, // historical_path_percentile (0)
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
							),
						),
						// Yields 25 + 11 = 36 values
					),
					// Stage 3: Ratios and Divergences
					data.NewValue[core.Primitive](
						data.NewSlice(0, 36),
						nomagique.NewNumber(
							data.NewSelect(
								8, 25, // notional_rate_ratio = notional_rate / baseline
								8, 25, // notional_rate_divergence = notional_rate - baseline
								22, 27, // spread_ratio = relative_spread / baseline
								22, 27, // spread_divergence = relative_spread - baseline
								13, 29, // midpoint_return_divergence = midpoint_log_return - baseline
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
							),
						),
						// Yields 36 + 5 = 41 values
					),
					// Stage 4: Z-scores
					data.NewValue[core.Primitive](
						data.NewSlice(0, 41),
						nomagique.NewNumber(
							data.NewSelect(
								37, 26, // notional_rate_zscore = divergence / scale
								39, 28, // spread_zscore = divergence / scale
								40, 30, // midpoint_return_zscore = divergence / scale
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
						// Yields 41 + 3 = 44 values
					),
					// Stage 5: Select exact outputKeys (40 values) + outBarStartNanos (1 value)
					data.NewSelect(
						23, // trade_price
						24, // trade_quantity
						0,  // trade_notional
						1,  // trade_interval_seconds
						2,  // volume_bar_target_quantity
						3,  // volume_bar_quantity
						4,  // volume_bar_notional
						5,  // volume_bar_trade_count
						6,  // volume_bar_duration
						7,  // volume_rate
						8,  // notional_rate
						9,  // trade_rate
						10, // completed_bars
						25, // notional_rate_baseline
						36, // notional_rate_ratio
						37, // notional_rate_divergence
						41, // notional_rate_zscore
						31, // notional_rate_velocity
						18, // best_bid
						19, // best_ask
						20, // midpoint
						21, // spread
						22, // relative_spread
						27, // relative_spread_baseline
						38, // spread_ratio
						39, // spread_divergence
						42, // spread_zscore
						32, // spread_divergence_velocity
						11, // midpoint:from
						12, // midpoint:at
						13, // midpoint_log_return
						14, // midpoint_return_rate
						15, // positive_midpoint_return
						16, // negative_midpoint_return
						29, // midpoint_return_baseline
						40, // midpoint_return_divergence
						43, // midpoint_return_zscore
						33, // midpoint_return_velocity
						34, // historical_path_distance
						35, // historical_path_percentile
						17, // outBarStartNanos
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "pumpdump_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", nil))
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		signal.Error(errnie.Err(errnie.Validation, "[pumpdump] price is required", nil))
		return nil
	}
	price := priceEntry.Metric.Raw

	qtyEntry := data.Pull(prior.Read("qty"))
	if qtyEntry == nil || qtyEntry.Metric == nil || qtyEntry.Metric.Raw <= 0 {
		return nil
	}
	qty := qtyEntry.Metric.Raw

	var bid, ask float64
	var hasBook bool

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
		ask = kraken.Float64(a.Price)
		hasBook = true
	})

	var midpoint, spread, relativeSpread float64
	if hasBook {
		touch := map[string]float64{
			"bid": bid,
			"ask": ask,
		}

		for _, value := range touch {
			if !broker.ValidTouchValue(value) {
				signal.Error(broker.InvalidTouch("pumpdump", prior.Label, touch))
				return nil
			}
		}

		if ask <= bid {
			signal.Error(broker.CrossedTouch("pumpdump", prior.Label, bid, ask))
			return nil
		}

		midpoint = (bid + ask) / 2.0
		spread = ask - bid
		if midpoint > 0 {
			relativeSpread = spread / midpoint
		}
	}

	atNanos := float64(prior.At.UnixNano())
	raws := []float64{price, qty, atNanos, midpoint}
	touchFacts := []float64{bid, ask, midpoint, spread, relativeSpread}
	tradeFacts := []float64{price, qty}

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&raws),
				unsafe.Pointer(&touchFacts),
				unsafe.Pointer(&tradeFacts),
			),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index < len(outputKeys) {
			output[outputKeys[index]] = *(*float64)(ptr)
		} else if index == len(outputKeys) {
			outBarStart := *(*float64)(ptr)
			if outBarStart > 0 {
				prior.From = time.Unix(0, int64(outBarStart))
			}
		}
		index++
	}

	return prior.Next(signal.Name(), output)
}
