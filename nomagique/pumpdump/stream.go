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
TouchGate validates and extracts executable touch quotes from measurements
or peers, deriving midpoint, spread, and relative spread.
*/
type TouchGate struct {
	err error
}

func NewTouchGate() core.Primitive {
	return &TouchGate{}
}

func (op *TouchGate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			bid := m.GetMetric("best_bid").Raw
			if bid == 0 {
				bid = m.GetMetric("bid").Raw
			}

			ask := m.GetMetric("best_ask").Raw
			if ask == 0 {
				ask = m.GetMetric("ask").Raw
			}

			if bid <= 0 || ask <= 0 || bid >= ask {
				m.Err = errnie.Err(
					errnie.Internal,
					fmt.Sprintf("pumpdump: invalid or crossed touch (bid=%f, ask=%f)", bid, ask),
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

			m.WriteMetric("best_bid", bid)
			m.WriteMetric("best_ask", ask)
			m.WriteMetric("midpoint", midpoint)
			m.WriteMetric("spread", spread)
			m.WriteMetric("relative_spread", relativeSpread)

			// Natural center and scale: prices centered at midpoint, scaled by spread
			m.SetCenterScale("best_bid", midpoint, spread)
			m.SetCenterScale("best_ask", midpoint, spread)
			m.SetCenterScale("midpoint", midpoint, spread)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *TouchGate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
BookTouch extracts authoritative executable touch quotes from the broker BookSource.
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

			m.WriteMetric("best_bid", bid)
			m.WriteMetric("best_ask", ask)
			m.WriteMetric("midpoint", midpoint)
			m.WriteMetric("spread", spread)
			m.WriteMetric("relative_spread", relativeSpread)

			m.SetCenterScale("best_bid", midpoint, spread)
			m.SetCenterScale("best_ask", midpoint, spread)
			m.SetCenterScale("midpoint", midpoint, spread)

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
			if bid > 0 && ask > bid {
				mid = (bid + ask) / 2.0
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

			m.WriteMetric("trade_price", price)
			m.WriteMetric("trade_quantity", qty)
			m.WriteMetric("trade_notional", notional)

			if hasInterval {
				m.WriteMetric("trade_interval_seconds", interval)
			}

			if hasInterval && duration > 0 && state.barQty >= state.targetQty {
				m.WriteMetric("volume_bar_target_quantity", state.targetQty)
				m.WriteMetric("volume_bar_quantity", state.barQty)
				m.WriteMetric("volume_bar_notional", state.barNotional)
				m.WriteMetric("volume_bar_trade_count", state.barTradeCount)
				m.WriteMetric("volume_bar_duration", duration)

				m.WriteMetric("volume_rate", state.barQty/duration)
				m.WriteMetric("notional_rate", state.barNotional/duration)
				m.WriteMetric("trade_rate", state.barTradeCount/duration)

				if state.barFromMidpoint > 0 && mid > 0 {
					m.WriteMetric("midpoint:from", state.barFromMidpoint)
					m.WriteMetric("midpoint:at", mid)

					logReturn := math.Log(mid / state.barFromMidpoint)
					m.WriteMetric("midpoint_log_return", logReturn)
					m.WriteMetric("midpoint_return_rate", logReturn/duration)

					if logReturn >= 0 {
						m.WriteMetric("positive_midpoint_return", logReturn)
						m.WriteMetric("negative_midpoint_return", 0.0)
					}

					if logReturn < 0 {
						m.WriteMetric("positive_midpoint_return", 0.0)
						m.WriteMetric("negative_midpoint_return", logReturn)
					}
				}

				state.completedBars++
				m.WriteMetric("completed_bars", state.completedBars)

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
