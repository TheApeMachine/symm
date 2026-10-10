package volumeclock

import "sort"

/*
Retained bounds the prior trade-quantity distribution the clock takes its
median from, matching the retention of the signals' other histories.
*/
const Retained = 256

/*
Bar is one closed volume bar. Target Q* was fixed when the bar opened, from
the median of the prior retained trade quantities.
*/
type Bar struct {
	Target, Quantity, Notional, Trades float64
	StartNanos, Duration               float64
	FromMid, AtMid                     float64
}

/*
Tick is what one trade did to the clock. Counted reports whether the trade
joined a bar (the first trade only seeds the quantity distribution); when
Closed is true, the trade completed Bar, which includes it.
*/
type Tick struct {
	Interval    float64
	HasInterval bool
	Counted     bool
	Closed      bool
	Bar         Bar
}

/*
Clock is the volume clock shared by the signals that window on volume, so a
bar means the same thing in each of them.
*/
type Clock struct {
	quantities    []float64
	bar           Bar
	open          bool
	CompletedBars float64
	hasPrev       bool
	prevAtNanos   float64
}

/*
Step adds one trade to the clock. Every trade counts, including trades
sharing a timestamp; a bar only closes once it has reached its target with
positive elapsed duration. Without a prior quantity distribution no bar
opens: the first trade only seeds it.
*/
func (clock *Clock) Step(price, qty, atNanos, midpoint float64) (tick Tick) {
	if clock.hasPrev && atNanos > clock.prevAtNanos {
		tick.Interval = (atNanos - clock.prevAtNanos) / 1e9
		tick.HasInterval = true
	}

	clock.hasPrev = true
	clock.prevAtNanos = atNanos

	if !clock.open && len(clock.quantities) > 0 {
		clock.openAt(atNanos, midpoint)
	}

	if clock.open {
		tick.Counted = true
		clock.bar.Quantity += qty
		clock.bar.Notional += price * qty
		clock.bar.Trades++

		if clock.bar.Quantity >= clock.bar.Target && atNanos > clock.bar.StartNanos {
			tick.Closed = true
			tick.Bar = clock.bar
			tick.Bar.Duration = (atNanos - clock.bar.StartNanos) / 1e9
			tick.Bar.AtMid = midpoint
			clock.CompletedBars++

			// The next bar opens where this one closed, on the
			// distribution that now includes this trade.
			clock.quantities = append(clock.quantities, qty)
			clock.retain()
			clock.openAt(atNanos, midpoint)

			return tick
		}
	}

	clock.quantities = append(clock.quantities, qty)
	clock.retain()

	return tick
}

func (clock *Clock) openAt(atNanos, midpoint float64) {
	clock.open = true
	clock.bar = Bar{
		Target:     median(clock.quantities),
		StartNanos: atNanos,
		FromMid:    midpoint,
	}
}

func (clock *Clock) retain() {
	if len(clock.quantities) > Retained {
		clock.quantities = clock.quantities[len(clock.quantities)-Retained:]
	}
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[mid]
	}

	return (sorted[mid-1] + sorted[mid]) / 2
}
