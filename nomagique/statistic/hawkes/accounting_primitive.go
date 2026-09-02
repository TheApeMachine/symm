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
	SymbolFromUnixSec     = types.MustIntern("hawkes/obs/from_unix_sec")
	SymbolFromUnixNsec    = types.MustIntern("hawkes/obs/from_unix_nsec")
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

	fromNanos := eventNanos(input)
	retainedCount := ArrivalsSeries.CountPtr(input)

	if retainedCount > 0 {
		var found bool
		fromNanos, _, found = ArrivalsSeries.Sample(input, 0)

		if !found {
			input.Err = fmt.Errorf("hawkes: retained observation origin is unavailable")

			return
		}
	}

	fromSec := float64(fromNanos) * 1e-9
	atSec := eventHorizonSec(input)
	span := atSec - fromSec

	input.Put(SymbolEventCount, count)
	input.Put(SymbolEventCountBuy, countBuy)
	input.Put(SymbolEventCountSell, countSell)
	input.Put(SymbolEventFracBuy, countBuy/count)
	input.Put(SymbolEventFracSell, countSell/count)
	input.Put(SymbolFromUnixSec, float64(fromNanos/1_000_000_000))
	input.Put(SymbolFromUnixNsec, float64(fromNanos%1_000_000_000))

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
