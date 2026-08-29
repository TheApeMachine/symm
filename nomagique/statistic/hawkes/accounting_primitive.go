package hawkes

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Accounting-scoped Frame facts: pure empirical counts and rates, per
signal/hawkes/README.md sections 6-7. These are the observed N/T quantities
and are never conflated with fitted mu (background_rate) — see
intensity_primitive.go's ConditionalIntensity for the fitted quantity.
*/
var (
	SymbolEventCount      = types.MustIntern("hawkes/obs/event_count")
	SymbolEventCountBuy   = types.MustIntern("hawkes/obs/event_count_buy")
	SymbolEventCountSell  = types.MustIntern("hawkes/obs/event_count_sell")
	SymbolEventFracBuy    = types.MustIntern("hawkes/obs/event_fraction_buy")
	SymbolEventFracSell   = types.MustIntern("hawkes/obs/event_fraction_sell")
	SymbolArrivalRateBuy  = types.MustIntern("hawkes/obs/arrival_rate_buy")
	SymbolArrivalRateSell = types.MustIntern("hawkes/obs/arrival_rate_sell")
	SymbolArrivalRate     = types.MustIntern("hawkes/obs/arrival_rate")
	SymbolFromSec         = types.MustIntern("hawkes/obs/from_sec")
	SymbolAtSec           = types.MustIntern("hawkes/obs/at_sec")
	SymbolMaturity        = types.MustIntern("hawkes/obs/maturity")
)

/*
Accounting reports empirical event counts, mark composition, and arrival
rates over the retained observation span [From, At]. This primitive requires
no fitted model: it is defined from the very first event, unlike every
model-dependent primitive in this package.
*/
func Accounting(input *types.Frame) {
	mark, hasMark := input.Get(SymbolMark)

	if !hasMark || mark == 0 || !finite(mark) {
		input.Err = fmt.Errorf("hawkes: a finite non-zero mark is required")

		return
	}

	buy, sell, hasHistory := retainedArrivals(input)

	if !hasHistory {
		buy, sell = nil, nil
	}

	countBuy := float64(len(buy))
	countSell := float64(len(sell))

	if mark > 0 {
		countBuy++
	} else {
		countSell++
	}

	count := countBuy + countSell

	fromSec := eventHorizonSec(input)

	if len(buy)+len(sell) > 0 {
		fromSec = earliestArrival(buy, sell)
	}

	atSec := eventHorizonSec(input)
	span := atSec - fromSec

	input.Put(SymbolEventCount, count)
	input.Put(SymbolEventCountBuy, countBuy)
	input.Put(SymbolEventCountSell, countSell)
	input.Put(SymbolEventFracBuy, countBuy/count)
	input.Put(SymbolEventFracSell, countSell/count)
	input.Put(SymbolFromSec, fromSec)
	input.Put(SymbolAtSec, atSec)

	nEffective := count
	maturity := 0.0

	if nEffective > 1 {
		maturity = 1 - 1/nEffective
	}

	input.Put(SymbolMaturity, maturity)

	if span <= 0 {
		return
	}

	rateBuy := countBuy / span
	rateSell := countSell / span

	input.Put(SymbolArrivalRateBuy, rateBuy)
	input.Put(SymbolArrivalRateSell, rateSell)
	input.Put(SymbolArrivalRate, rateBuy+rateSell)
}

func earliestArrival(buy, sell []float64) float64 {
	switch {
	case len(buy) == 0:
		return sell[0]
	case len(sell) == 0:
		return buy[0]
	case sell[0] < buy[0]:
		return sell[0]
	default:
		return buy[0]
	}
}
