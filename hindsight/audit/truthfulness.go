package audit

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
truthCheck names one recomputation the truthfulness audit performs. A check
that found nothing to compare is reported, so a metric name that no producer
emits shows up as zero comparisons rather than as a silent pass.
*/
var truthChecks = []string{
	"trade_fields",                // price > 0, qty > 0, side buy|sell
	"cvd_coverage",                // one cvd frame per stored trade
	"response_midpoint:at",        // == this trade's price
	"response_midpoint:from",      // == the previous trade's price
	"midpoint_log_return",         // == ln(price / previous price)
	"trade_rate",                  // == 1 / (At - previous At)
	"buy_notional_rate",           // == buy notional / dt
	"sell_notional_rate",          // == sell notional / dt
	"rate_without_elapsed_time",   // at most one rated frame among same-time trades
	"signed_net_fraction_bounded", // within [-1, 1]
}

/*
AnalyzeTruthfulness recomputes, from the stored spot:trade tape, what cvd
must have published for each trade, and compares it with the stored cvd frame
of that trade. Trades and cvd frames are paired per symbol by event time; a
time shared by several trades is ambiguous and skipped (counted). cvd keeps
its previous price and time per symbol and epoch, so the previous trade is the
previous stored trade of the same symbol.
*/
func AnalyzeTruthfulness(trades []*data.Measurement, measurements []*data.Measurement) TruthfulnessAudit {
	compared := make(map[string]int, len(truthChecks))
	violations := make(map[string]int, len(truthChecks))
	discrepancies := make([]MetricDiscrepancy, 0)

	record := func(check string, observed, expected float64, ok bool) {
		compared[check]++

		if ok {
			return
		}

		violations[check]++

		if len(discrepancies) < 200 {
			discrepancies = append(discrepancies, MetricDiscrepancy{
				Signal: "cvd", Metric: check, ObservedVal: observed, ExpectedVal: expected,
				Error: math.Abs(observed - expected),
			})
		}
	}

	equal := func(observed, expected float64) bool {
		return math.Abs(observed-expected) <= 1e-9*math.Max(math.Abs(expected), math.Abs(observed))
	}

	cvdByKey := make(map[string][]*data.Measurement)
	// cvd handles a symbol's trades in order, so a trade later than the
	// symbol's last loaded cvd frame lies past the audit sample, not missing.
	lastCVD := make(map[string]int64)

	for _, m := range measurements {
		if m != nil && m.Source == "cvd" {
			lastCVD[m.Label] = max(lastCVD[m.Label], m.At.UnixNano())
			key := fmt.Sprintf("%s|%d", m.Label, m.At.UnixNano())
			cvdByKey[key] = append(cvdByKey[key], m)

			for entry := range m.Read() {
				if entry != nil && entry.Metric != nil && entry.Key == "signed_net_fraction" {
					value := entry.Metric.Raw
					record("signed_net_fraction_bounded", value, 1, value >= -1 && value <= 1)
				}
			}
		}
	}

	bySymbol := make(map[string][]*data.Measurement)

	for _, trade := range trades {
		if trade != nil && trade.Source == "spot:trade" {
			bySymbol[trade.Label] = append(bySymbol[trade.Label], trade)
		}
	}

	ambiguous := 0
	totalTrades := 0

	for symbol, tape := range bySymbol {
		slices.SortStableFunc(tape, func(left, right *data.Measurement) int {
			if order := left.At.Compare(right.At); order != 0 {
				return order
			}

			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		sameAt := make(map[int64]int)

		for _, trade := range tape {
			sameAt[trade.At.UnixNano()]++
		}

		var previous *data.Measurement
		checkedAt := make(map[int64]bool)

		for _, trade := range tape {
			totalTrades++
			price, qty := tradeField(trade, "price"), tradeField(trade, "qty")
			side := trade.Meta("side")
			record("trade_fields", price, qty, price > 0 && qty > 0 && (side == "buy" || side == "sell"))

			at := trade.At.UnixNano()
			prior := previous
			previous = trade

			if at > lastCVD[symbol] {
				continue
			}

			if sameAt[at] > 1 {
				ambiguous++

				// Of k trades sharing one time, at most the first has elapsed
				// time since its predecessor; any further frame carrying a
				// rate divided by an interval the clock never showed.
				if !checkedAt[at] {
					checkedAt[at] = true
					rated := 0

					for _, frame := range cvdByKey[fmt.Sprintf("%s|%d", symbol, at)] {
						for entry := range frame.Read() {
							if entry != nil && entry.Metric != nil && entry.Key == "trade_rate" {
								rated++
							}
						}
					}

					record("rate_without_elapsed_time", float64(rated), 1, rated <= 1)
				}

				continue
			}

			frames := cvdByKey[fmt.Sprintf("%s|%d", symbol, at)]
			record("cvd_coverage", float64(len(frames)), 1, len(frames) == 1)

			if len(frames) != 1 {
				continue
			}

			published := make(map[string]float64)

			for entry := range frames[0].Read() {
				if entry != nil && entry.Metric != nil {
					published[entry.Key] = entry.Metric.Raw
				}
			}

			if prior == nil {
				continue
			}

			priorPrice := tradeField(prior, "price")

			if value, ok := published["response_midpoint:at"]; ok {
				record("response_midpoint:at", value, price, equal(value, price))
			}

			if value, ok := published["response_midpoint:from"]; ok {
				record("response_midpoint:from", value, priorPrice, equal(value, priorPrice))
			}

			if value, ok := published["midpoint_log_return"]; ok && price > 0 && priorPrice > 0 {
				want := math.Log(price / priorPrice)
				record("midpoint_log_return", value, want, math.Abs(value-want) <= 1e-12+1e-9*math.Abs(want))
			}

			dt := trade.At.Sub(prior.At).Seconds()
			rates := map[string]float64{"trade_rate": 1}

			if side == "buy" {
				rates["buy_notional_rate"], rates["sell_notional_rate"] = price*qty, 0
			} else {
				rates["buy_notional_rate"], rates["sell_notional_rate"] = 0, price*qty
			}

			for name, numerator := range rates {
				value, ok := published[name]

				if !ok {
					continue
				}

				record(name, value, numerator/dt, equal(value, numerator/dt))
			}
		}
	}

	totalViolations := 0
	unexercised := make([]string, 0)

	for _, check := range truthChecks {
		totalViolations += violations[check]

		if compared[check] == 0 && check != "rate_without_elapsed_time" {
			unexercised = append(unexercised, check)
		}
	}

	verdict := VerdictValid

	switch {
	case totalViolations > 0:
		verdict = VerdictBreach
	case totalTrades == 0 || len(unexercised) > 0:
		verdict = VerdictInsufficient
	}

	summary := fmt.Sprintf(
		"Metric Truthfulness: %d stored trades recomputed against cvd (%d skipped as same-time ambiguous). "+
			"Violations=%d by check %v; comparisons %v; unexercised checks %v.",
		totalTrades, ambiguous, totalViolations, violations, compared, unexercised,
	)

	return TruthfulnessAudit{
		TotalChecked:       totalTrades,
		ViolationsCount:    totalViolations,
		SyntheticTimeSteps: violations["rate_without_elapsed_time"],
		Comparisons:        compared,
		ViolationsByCheck:  violations,
		AmbiguousTrades:    ambiguous,
		UnexercisedChecks:  unexercised,
		Discrepancies:      discrepancies,
		SummaryText:        summary,
		Status:             verdict,
		Passed:             passed(verdict),
	}
}

func tradeField(trade *data.Measurement, key string) float64 {
	for entry := range trade.Read() {
		if entry != nil && entry.Metric != nil && entry.Key == key {
			return entry.Metric.Raw
		}
	}

	return 0
}
