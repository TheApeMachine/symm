package liquidity

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Gate validates that the incoming measurement carries finite, positive bid/ask prices
and touch quantities with ask > bid.
*/
type Gate struct {
	err error
}

func NewGate() core.Primitive {
	return &Gate{}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**data.Measurement)(arriving)

			if measurement == nil {
				continue
			}

			bidMetric, hasBid := measurement.LookupMetric("bid")
			askMetric, hasAsk := measurement.LookupMetric("ask")
			bidQtyMetric, hasBidQty := measurement.LookupMetric("bid_qty")
			askQtyMetric, hasAskQty := measurement.LookupMetric("ask_qty")

			if !hasBid || !hasAsk || !hasBidQty || !hasAskQty {
				measurement.Err = fmt.Errorf("%w: liquidity: bid, ask, bid_qty, ask_qty required", core.ErrDomain)
				if !yield(arriving) {
					return
				}
				continue
			}

			bid := bidMetric.Raw
			ask := askMetric.Raw
			bidQty := bidQtyMetric.Raw
			askQty := askQtyMetric.Raw

			if bid <= 0 || ask <= 0 || bidQty <= 0 || askQty <= 0 ||
				math.IsNaN(bid) || math.IsNaN(ask) || math.IsNaN(bidQty) || math.IsNaN(askQty) ||
				math.IsInf(bid, 0) || math.IsInf(ask, 0) || math.IsInf(bidQty, 0) || math.IsInf(askQty, 0) {
				measurement.Err = fmt.Errorf("%w: liquidity: finite positive prices and displayed quantities required", core.ErrDomain)
				if !yield(arriving) {
					return
				}
				continue
			}

			if ask <= bid {
				measurement.Err = fmt.Errorf("%w: liquidity: positive order violated (%f <= %f)", core.ErrDomain, ask, bid)
				if !yield(arriving) {
					return
				}
				continue
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	return op.err
}

/*
Touch derives touch observables: notional per side, midpoint, spread, relative spread,
and scale-free touch imbalance.
*/
type Touch struct {
	err error
}

func NewTouch() core.Primitive {
	return &Touch{}
}

func (op *Touch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**data.Measurement)(arriving)

			if measurement == nil || measurement.Err != nil {
				if !yield(arriving) {
					return
				}
				continue
			}

			bid := measurement.GetMetric("bid").Raw
			ask := measurement.GetMetric("ask").Raw
			bidQty := measurement.GetMetric("bid_qty").Raw
			askQty := measurement.GetMetric("ask_qty").Raw

			bidNotional := bid * bidQty
			askNotional := ask * askQty
			midpoint := (bid + ask) / 2.0
			spread := ask - bid
			relative := spread / midpoint

			totalTouchQty := bidQty + askQty
			totalTouchNotional := bidNotional + askNotional

			measurement.SetMetric("best_bid_price", data.NewMetric(
				"best_bid_price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(bid))
			measurement.SetMetric("best_ask_price", data.NewMetric(
				"best_ask_price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(ask))
			measurement.SetMetric("touch_quantity:bid", data.NewMetric(
				"touch_quantity:bid",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchQty,
			).Write(bidQty))
			measurement.SetMetric("touch_quantity:ask", data.NewMetric(
				"touch_quantity:ask",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchQty,
			).Write(askQty))
			measurement.SetMetric("touch_notional:bid", data.NewMetric(
				"touch_notional:bid",
				data.UnitNotional,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchNotional,
			).Write(bidNotional))
			measurement.SetMetric("touch_notional:ask", data.NewMetric(
				"touch_notional:ask",
				data.UnitNotional,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchNotional,
			).Write(askNotional))
			measurement.SetMetric("midpoint", data.NewMetric(
				"midpoint",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(midpoint))
			measurement.SetMetric("spread", data.NewMetric(
				"spread",
				data.UnitSpread,
				data.TimescaleInstantaneous,
				0.0,
				spread,
			).Write(spread))
			measurement.SetMetric("relative_spread", data.NewMetric(
				"relative_spread",
				data.UnitRelativeSpread,
				data.TimescaleInstantaneous,
				0.0,
				relative,
			).Write(relative))
			measurement.SetMetric("two_sided_touch_notional", data.NewMetric(
				"two_sided_touch_notional",
				data.UnitNotional,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchNotional,
			).Write(math.Min(bidNotional, askNotional)))

			if totalTouchNotional > 0 {
				imbalance := (bidNotional - askNotional) / totalTouchNotional
				measurement.SetMetric("touch_notional_imbalance", data.NewMetric(
					"touch_notional_imbalance",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write(imbalance))
			}

			if bidNotional > 0 {
				measurement.SetMetric("_log_bid_notional", data.NewMetric(
					"_log_bid_notional",
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
					math.Log(totalTouchNotional),
					1.0,
				).Write(math.Log(bidNotional)))
			}
			if askNotional > 0 {
				measurement.SetMetric("_log_ask_notional", data.NewMetric(
					"_log_ask_notional",
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
					math.Log(totalTouchNotional),
					1.0,
				).Write(math.Log(askNotional)))
			}
			if relative > 0 {
				measurement.SetMetric("_log_relative_spread", data.NewMetric(
					"_log_relative_spread",
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
					math.Log(relative),
					1.0,
				).Write(math.Log(relative)))
			}

			measurement.EnsureMetadata()

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Touch) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	return op.err
}
