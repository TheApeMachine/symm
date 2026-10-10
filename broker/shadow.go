package broker

import (
	"sync"
	"time"
)

/*
ShadowFill is one order walked against the as-of book version instead of the
venue's simulated fill. Short is the requested quantity beyond the displayed
depth left after earlier shadow fills; it is not filled. Defined is false when
no verified book version covered the fill time.
*/
type ShadowFill struct {
	Quantity float64
	Gross    float64
	Short    float64
	At       time.Time
	Defined  bool
}

/*
VWAP is the shadow fill's average price.
*/
func (fill ShadowFill) VWAP() float64 {
	if fill.Quantity <= 0 {
		return 0
	}

	return fill.Gross / fill.Quantity
}

/*
Shadow is the evaluation ledger's fill model. The paper venue fills market
orders at the top of the ticker for any size and never removes liquidity, so
every venue fill is also walked level by level through the book version in
effect at its fill time. What a shadow fill takes is subtracted from those
levels for later fills until the level refreshes: a version shows it at a
quantity other than the one recorded when it was taken from, or no longer
shows it, meaning the venue updated it. There is no queue model.
*/
type Shadow struct {
	mu    sync.Mutex
	taken map[string]map[float64]consumption
}

type consumption struct {
	displayed float64
	consumed  float64
}

func NewShadow() *Shadow {
	return &Shadow{taken: make(map[string]map[float64]consumption)}
}

/*
Fill walks quantity through view's side for side (asks for a buy, bids for a
sell) and records what it takes.
*/
func (shadow *Shadow) Fill(symbol string, side Direction, view *BookView, quantity float64) ShadowFill {
	return shadow.walk(symbol, side, view, quantity, true)
}

/*
Quote walks quantity the same way without taking anything, to mark open
quantity.
*/
func (shadow *Shadow) Quote(symbol string, side Direction, view *BookView, quantity float64) ShadowFill {
	return shadow.walk(symbol, side, view, quantity, false)
}

func (shadow *Shadow) walk(
	symbol string, side Direction, view *BookView, quantity float64, record bool,
) ShadowFill {
	if view == nil {
		return ShadowFill{Short: quantity}
	}

	levels := view.Bids

	if side == BUY {
		levels = view.Asks
	}

	shadow.mu.Lock()
	defer shadow.mu.Unlock()

	key := symbol + "|" + string(side)
	prior := shadow.taken[key]
	kept := make(map[float64]consumption)
	fill := ShadowFill{At: view.At, Defined: true}
	remaining := quantity

	for _, level := range levels {
		available := level.Quantity
		taken, held := prior[level.Price]

		if held && taken.displayed == level.Quantity {
			available = max(available-taken.consumed, 0)
			kept[level.Price] = taken
		}

		take := min(available, remaining)

		if take <= 0 {
			continue
		}

		fill.Quantity += take
		fill.Gross += take * level.Price
		remaining -= take

		taken = kept[level.Price]
		taken.displayed = level.Quantity
		taken.consumed += take
		kept[level.Price] = taken
	}

	fill.Short = max(remaining, 0)

	if record {
		shadow.taken[key] = kept
	}

	return fill
}

/*
Adjusted returns view with what earlier shadow fills took from levels that
have not refreshed subtracted, the book our own paper orders would have left.
Levels emptied by it are dropped. view itself is never modified.
*/
func (shadow *Shadow) Adjusted(symbol string, view *BookView) *BookView {
	if view == nil {
		return nil
	}

	shadow.mu.Lock()
	defer shadow.mu.Unlock()

	adjusted := *view
	adjusted.Bids = shadow.remaining(symbol, SELL, view.Bids)
	adjusted.Asks = shadow.remaining(symbol, BUY, view.Asks)

	return &adjusted
}

/*
remaining is levels after unrefreshed consumption by side. The caller holds
the lock.
*/
func (shadow *Shadow) remaining(symbol string, side Direction, levels []BookLevel) []BookLevel {
	prior := shadow.taken[symbol+"|"+string(side)]

	if len(prior) == 0 {
		return levels
	}

	out := make([]BookLevel, 0, len(levels))

	for _, level := range levels {
		if taken, held := prior[level.Price]; held && taken.displayed == level.Quantity {
			level.Quantity -= taken.consumed
		}

		if level.Quantity > 0 {
			out = append(out, level)
		}
	}

	return out
}
