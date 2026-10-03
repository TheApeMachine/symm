package pumpdump

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"time"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
BookTouch extracts authoritative executable touch quotes from the broker BookSource.
Every metric written explicitly registers its label, unit, timescale, center, and scale.
Prices are centered at midpoint and scaled by spread.
*/
type BookTouch struct {
	err   error
	books broker.BookSource
}

func NewBookTouch(books broker.BookSource) core.Primitive {
	return &BookTouch{books: books}
}

func (op *BookTouch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			var bid, ask float64
			if op.books != nil {
				op.books.Book(m.Label, func(b *spotbook.Book) {
					if b == nil {
						return
					}

					if bestBid := b.BestBid(); bestBid != nil && bestBid.Price != nil {
						bid = bestBid.Price.Float64()
					}

					if bestAsk := b.BestAsk(); bestAsk != nil && bestAsk.Price != nil {
						ask = bestAsk.Price.Float64()
					}
				})
			}

			if bid == 0 {
				bid = m.GetMetric("best_bid").Raw
				if bid == 0 {
					bid = m.GetMetric("bid").Raw
				}
			}
			if ask == 0 {
				ask = m.GetMetric("best_ask").Raw
				if ask == 0 {
					ask = m.GetMetric("ask").Raw
				}
			}

			if bid <= 0 || ask <= 0 || bid >= ask {
				m.Err = errnie.Err(
					errnie.Internal,
					fmt.Sprintf("pumpdump: invalid or crossed book touch (bid=%f, ask=%f)", bid, ask),
					nil,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			spread := ask - bid
			midpoint := (bid + ask) / 2.0
			relativeSpread := spread / midpoint

			m.SetMetric("best_bid", data.NewMetric[float64](
				"best_bid",
				data.UnitPrice,
				data.TimescaleTick,
				midpoint,
				spread,
			).Write(bid))

			m.SetMetric("best_ask", data.NewMetric[float64](
				"best_ask",
				data.UnitPrice,
				data.TimescaleTick,
				midpoint,
				spread,
			).Write(ask))

			m.SetMetric("midpoint", data.NewMetric[float64](
				"midpoint",
				data.UnitPrice,
				data.TimescaleTick,
				midpoint,
				spread,
			).Write(midpoint))

			m.SetMetric("spread", data.NewMetric[float64](
				"spread",
				data.UnitSpread,
				data.TimescaleTick,
				spread,
				spread,
			).Write(spread))

			m.SetMetric("relative_spread", data.NewMetric[float64](
				"relative_spread",
				data.UnitRelativeSpread,
				data.TimescaleTick,
				relativeSpread,
				relativeSpread,
			).Write(relativeSpread))

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *BookTouch) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
VolumeClockState maintains causal volume bar aggregation and robust target quantity.
*/
type VolumeClockState struct {
	hasTrade        bool
	prevTradeTime   time.Time
	barStartTime    time.Time
	targetQty       float64
	tradeCount      float64
	barQty          float64
	barNotional     float64
	barTradeCount   float64
	completedBars   float64
	barFromMidpoint float64
}

/*
VolumeClock measures tape trades on an adaptive volume clock.
Every metric written explicitly registers its label, unit, timescale, center, and scale.
*/
type VolumeClock struct {
	err    error
	states map[string]*VolumeClockState
}

func NewVolumeClock() core.Primitive {
	return &VolumeClock{
		states: make(map[string]*VolumeClockState),
	}
}

func (op *VolumeClock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			priceMetric, hasPrice := m.LookupMetric("price")
			qtyMetric, hasQty := m.LookupMetric("qty")

			if !hasPrice || !hasQty || priceMetric.Raw <= 0 || qtyMetric.Raw <= 0 {
				m.Err = errnie.Err(
					errnie.Validation,
					"pumpdump: positive price and quantity required",
					nil,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			state, exists := op.states[m.Label]
			if !exists {
				state = &VolumeClockState{}
				op.states[m.Label] = state
			}

			price := priceMetric.Raw
			qty := qtyMetric.Raw
			notional := price * qty

			var bid, ask float64
			if metric, ok := m.LookupMetric("best_bid"); ok {
				bid = metric.Raw
			}

			if bid == 0 {
				if metric, ok := m.LookupMetric("bid"); ok {
					bid = metric.Raw
				}
			}

			if metric, ok := m.LookupMetric("best_ask"); ok {
				ask = metric.Raw
			}

			if ask == 0 {
				if metric, ok := m.LookupMetric("ask"); ok {
					ask = metric.Raw
				}
			}

			mid := 0.0
			spread := 0.0
			if bid > 0 && ask > bid {
				mid = (bid + ask) / 2.0
				spread = ask - bid
			}

			if !state.hasTrade {
				state.targetQty = qty
				state.barStartTime = m.At
				if mid > 0 {
					state.barFromMidpoint = mid
				}
			} else {
				state.targetQty = (state.targetQty*state.tradeCount + qty) / (state.tradeCount + 1)
			}

			state.tradeCount++

			var interval float64
			var hasInterval bool

			if state.hasTrade {
				interval = m.At.Sub(state.prevTradeTime).Seconds()
				hasInterval = true
			}

			state.prevTradeTime = m.At
			state.hasTrade = true

			state.barQty += qty
			state.barNotional += notional
			state.barTradeCount++

			duration := m.At.Sub(state.barStartTime).Seconds()

			// Center and scale exist only against a valid book: the touch midpoint and spread are measured.
			m.SetMetric("trade_price", data.NewMetric[float64](
				"trade_price",
				data.UnitPrice,
				data.TimescaleTick,
				mid,
				spread,
			).Write(price))

			m.SetMetric("trade_quantity", data.NewMetric[float64](
				"trade_quantity",
				data.UnitQuantity,
				data.TimescaleTick,
				0.0,
				0.0,
			).Write(qty))

			m.SetMetric("trade_notional", data.NewMetric[float64](
				"trade_notional",
				data.UnitNotional,
				data.TimescaleTick,
				0.0,
				0.0,
			).Write(notional))

			if hasInterval {
				m.SetMetric("trade_interval_seconds", data.NewMetric[float64](
					"trade_interval_seconds",
					data.UnitSecond,
					data.TimescaleTick,
					0.0,
					0.0,
				).Write(interval))
			}

			if hasInterval && duration > 0 && state.barQty >= state.targetQty {
				m.SetMetric("volume_bar_target_quantity", data.NewMetric[float64](
					"volume_bar_target_quantity",
					data.UnitQuantity,
					data.TimescaleVolumeBar,
					0.0,
					0.0,
				).Write(state.targetQty))

				m.SetMetric("volume_bar_quantity", data.NewMetric[float64](
					"volume_bar_quantity",
					data.UnitQuantity,
					data.TimescaleVolumeBar,
					0.0,
					0.0,
				).Write(state.barQty))

				m.SetMetric("volume_bar_notional", data.NewMetric[float64](
					"volume_bar_notional",
					data.UnitNotional,
					data.TimescaleVolumeBar,
					0.0,
					0.0,
				).Write(state.barNotional))

				m.SetMetric("volume_bar_trade_count", data.NewMetric[float64](
					"volume_bar_trade_count",
					data.UnitCount,
					data.TimescaleVolumeBar,
					0.0,
					0.0,
				).Write(state.barTradeCount))

				m.SetMetric("volume_bar_duration", data.NewMetric[float64](
					"volume_bar_duration",
					data.UnitDuration,
					data.TimescaleVolumeBar,
					0.0,
					0.0,
				).Write(duration))

				m.SetMetric("volume_rate", data.NewMetric[float64](
					"volume_rate",
					data.UnitVolumeRate,
					data.TimescalePerSecond,
					0.0,
					0.0,
				).Write(state.barQty/duration))

				m.SetMetric("notional_rate", data.NewMetric[float64](
					"notional_rate",
					data.UnitNotionalRate,
					data.TimescalePerSecond,
					0.0,
					0.0,
				).Write(state.barNotional/duration))

				m.SetMetric("trade_rate", data.NewMetric[float64](
					"trade_rate",
					data.UnitTradeRate,
					data.TimescalePerSecond,
					0.0,
					0.0,
				).Write(state.barTradeCount/duration))

				if state.barFromMidpoint > 0 && mid > 0 {
					// Return resolution is the measured touch width relative to the midpoint.
					relSpread := spread / mid
					logReturn := math.Log(mid / state.barFromMidpoint)
					returnRateScale := relSpread / duration

					m.SetMetric("midpoint_log_return", data.NewMetric[float64](
						"midpoint_log_return",
						data.UnitLogReturn,
						data.TimescaleVolumeBar,
						0.0,
						relSpread,
					).Write(logReturn))

					m.SetMetric("midpoint_return_rate", data.NewMetric[float64](
						"midpoint_return_rate",
						data.UnitVelocity,
						data.TimescalePerSecond,
						0.0,
						returnRateScale,
					).Write(logReturn/duration))

					posReturn := 0.0
					negReturn := 0.0
					if logReturn > 0 {
						posReturn = logReturn
					}
					if logReturn < 0 {
						negReturn = logReturn
					}

					m.SetMetric("positive_midpoint_return", data.NewMetric[float64](
						"positive_midpoint_return",
						data.UnitLogReturn,
						data.TimescaleVolumeBar,
						0.0,
						relSpread,
					).Write(posReturn))

					m.SetMetric("negative_midpoint_return", data.NewMetric[float64](
						"negative_midpoint_return",
						data.UnitLogReturn,
						data.TimescaleVolumeBar,
						0.0,
						relSpread,
					).Write(negReturn))
				}

				state.completedBars++
				m.SetMetric("completed_bars", data.NewMetric[float64](
					"completed_bars",
					data.UnitCount,
					data.TimescaleSession,
					0.0,
					0.0,
				).Write(state.completedBars))

				// Reset the bar
				state.barQty = 0
				state.barNotional = 0
				state.barTradeCount = 0
				state.barStartTime = m.At
				state.barFromMidpoint = mid
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *VolumeClock) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
