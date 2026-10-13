package audit

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
	"level3_replay",               // every stored level3 entry replays and matches its venue checksum
	"trade_book",                  // book snapshot stamped on the trade == replayed book at its SeqIdx, every level exact
	"response_midpoint:at",        // == (best bid + best ask) / 2 of the snapshot stamped on this trade
	"response_midpoint:from",      // == response_midpoint:at of the previous trade's frame
	"midpoint_log_return",         // == ln(response_midpoint:at / response_midpoint:from)
	"trade_rate",                  // == 1 / (At - previous At)
	"buy_notional_rate",           // == buy notional / dt
	"sell_notional_rate",          // == sell notional / dt
	"rate_without_elapsed_time",   // at most one rated frame among same-time trades
	"signed_net_fraction_bounded", // within [-1, 1]
}

/*
AnalyzeTruthfulness recomputes, from the stored spot:trade and spot:level3
tapes, what cvd must have published for each trade, and compares it with the
stored cvd frame of that trade. Trades and cvd frames are paired per symbol by
event time; a time shared by several trades is ambiguous and skipped (counted).

Trades are walked in SeqIdx order, cvd's processing order, so the previous
trade is the one whose state cvd carried into this one. Ingress takes level3
entry sequence numbers under the book's write lock and trade sequence numbers,
with the book snapshot it stamps onto the trade, under its read lock; replaying
every stored entry sequenced before a trade through broker.Book therefore
rebuilds exactly the book that trade was stamped from. The stamped snapshot is
checked level by level, to the venue's exact digits, against that replay;
cvd's response_midpoint:at against the stamped touch's midpoint, :from against
the previous frame's :at, and the log return against ln(:at / :from).

A trade stamped without a book is not compared: the live book was unseeded or
stale at that event, a state the stored tape does not record.
*/
func AnalyzeTruthfulness(
	ctx context.Context,
	trades []*data.Measurement,
	level3 []*data.Measurement,
	measurements []*data.Measurement,
) TruthfulnessAudit {
	compared := make(map[string]int, len(truthChecks))
	violations := make(map[string]int, len(truthChecks))
	discrepancies := make([]MetricDiscrepancy, 0)

	record := func(signal, check string, observed, expected float64, ok bool) {
		compared[check]++

		if ok {
			return
		}

		errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[audit.truthfulness] discrepancy on %s: observed=%v expected=%v", check, observed, expected),
			nil,
		), "check", check, "observed", observed, "expected", expected)

		violations[check]++

		if len(discrepancies) < 200 {
			discrepancies = append(discrepancies, MetricDiscrepancy{
				Signal: signal, Metric: check, ObservedVal: observed, ExpectedVal: expected,
				Error: math.Abs(observed - expected),
			})
		}
	}

	equal := func(observed, expected float64) bool {
		if math.IsNaN(observed) || math.IsNaN(expected) || math.IsInf(observed, 0) || math.IsInf(expected, 0) {
			return false
		}

		return math.Abs(observed-expected) <= 1e-12+1e-9*math.Max(math.Abs(expected), math.Abs(observed))
	}

	cvdByKey := make(map[string][]*data.Measurement)
	// cvd handles a symbol's trades in order, so a trade later than the
	// symbol's last loaded cvd frame lies past the audit sample, not missing.
	lastCVD := make(map[string]int64)

	for _, measurement := range measurements {
		if measurement == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.truthfulness] nil measurement in audit stream",
				nil,
			))

			continue
		}

		if measurement.Source == "cvd" {
			lastCVD[measurement.Label] = max(lastCVD[measurement.Label], measurement.At.UnixNano())
			key := fmt.Sprintf("%s|%d", measurement.Label, measurement.At.UnixNano())
			cvdByKey[key] = append(cvdByKey[key], measurement)

			for entry := range measurement.Read() {
				if entry == nil || entry.Metric == nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"[audit.truthfulness] nil metric entry in cvd measurement",
						nil,
					), "label", measurement.Label)

					continue
				}

				if entry.Key == "signed_net_fraction" {
					value := entry.Metric.Raw()
					record("cvd", "signed_net_fraction_bounded", value, math.Max(-1, math.Min(1, value)), value >= -1 && value <= 1)
				}
			}
		}
	}

	bySymbol := make(map[string][]*data.Measurement)

	for _, trade := range trades {
		if trade == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.truthfulness] nil trade in audit stream",
				nil,
			))

			continue
		}

		if trade.Source == "spot:trade" {
			bySymbol[trade.Label] = append(bySymbol[trade.Label], trade)
		}
	}

	ambiguous := 0
	totalTrades := 0
	level3Entries := level3Frames(level3)
	book := broker.NewBook(ctx, spot.NewNormalizer())
	cursors := make(map[string]int, len(level3Entries))

	replay := func(symbol string, before int64) {
		frames := level3Entries[symbol]

		for cursors[symbol] < len(frames) && frames[cursors[symbol]].first < before {
			if ctx.Err() != nil {
				return
			}

			frame := frames[cursors[symbol]]
			err := book.ApplyFrame(frame.orders)
			passVal := 1.0

			if err != nil {
				passVal = 0.0
			}

			record("book", "level3_replay", passVal, 1.0, err == nil)
			cursors[symbol]++
		}
	}

	symbols := make([]string, 0, len(bySymbol))

	for sym := range bySymbol {
		symbols = append(symbols, sym)
	}

	slices.Sort(symbols)

	for _, symbol := range symbols {
		if ctx.Err() != nil {
			break
		}

		tape := bySymbol[symbol]

		slices.SortStableFunc(tape, func(left, right *data.Measurement) int {
			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		sameAt := make(map[int64]int)

		for _, trade := range tape {
			sameAt[trade.At.UnixNano()]++
		}

		var previous *data.Measurement
		var previousPublished map[string]float64
		checkedAt := make(map[int64]bool)

		for _, trade := range tape {
			if ctx.Err() != nil {
				break
			}

			replay(symbol, trade.SeqIdx)

			stamped, err := broker.ReadSnapshot(trade)

			if err != nil {
				errnie.Error(err, "symbol", symbol, "seq", trade.SeqIdx)
				record("book", "trade_book", 0.0, 1.0, false)
			}

			if err == nil && stamped != nil {
				replayed := book.Snapshot(symbol)
				isEqual := stamped.Equal(replayed)
				obsVal := 0.0

				if isEqual {
					obsVal = 1.0
				}

				record("book", "trade_book", obsVal, 1.0, isEqual)
			}

			price, qty := tradeField(trade, "price"), tradeField(trade, "qty")
			var side string

			if entry := data.Pull(trade.Meta("side")); entry != nil {
				side = entry.Value
			}
			validFields := price > 0 && qty > 0 && (side == "buy" || side == "sell")
			fieldVal := 0.0

			if validFields {
				fieldVal = 1.0
			}

			record("trade", "trade_fields", fieldVal, 1.0, validFields)

			at := trade.At.UnixNano()
			prior := previous
			previous = trade
			chain := previousPublished
			previousPublished = nil

			last, hasCVD := lastCVD[symbol]

			if hasCVD && at > last {
				continue
			}

			totalTrades++

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

					record("cvd", "rate_without_elapsed_time", float64(rated), 1.0, rated <= 1)
				}

				continue
			}

			frames := cvdByKey[fmt.Sprintf("%s|%d", symbol, at)]
			record("cvd", "cvd_coverage", float64(len(frames)), 1.0, len(frames) == 1)

			if len(frames) != 1 {
				continue
			}

			published := make(map[string]float64)

			for entry := range frames[0].Read() {
				if entry != nil && entry.Metric != nil {
					published[entry.Key] = entry.Metric.Raw()
				}
			}

			previousPublished = published

			if value, ok := published["response_midpoint:at"]; ok {
				if stamped == nil {
					record("cvd", "response_midpoint:at", value, 0.0, false)
				}

				if stamped != nil {
					bid, ask := stamped.Touch()
					want := (bid.Price.Float64() + ask.Price.Float64()) / 2.0
					record("cvd", "response_midpoint:at", value, want, equal(value, want))
				}
			}

			if value, ok := published["response_midpoint:from"]; ok {
				if previousAt, chained := chain["response_midpoint:at"]; chained {
					record("cvd", "response_midpoint:from", value, previousAt, equal(value, previousAt))
				}
			}

			if value, ok := published["midpoint_log_return"]; ok {
				atMid, hasAt := published["response_midpoint:at"]
				fromMid, hasFrom := published["response_midpoint:from"]

				if !hasAt || !hasFrom || atMid <= 0 || fromMid <= 0 {
					record("cvd", "midpoint_log_return", value, 0.0, false)
				}

				if hasAt && hasFrom && atMid > 0 && fromMid > 0 {
					want := math.Log(atMid / fromMid)
					record("cvd", "midpoint_log_return", value, want, equal(value, want))
				}
			}

			if prior == nil {
				continue
			}

			timeDelta := trade.At.Sub(prior.At).Seconds()

			if timeDelta <= 0 {
				continue
			}

			notional := price * qty
			rates := map[string]float64{
				"trade_rate":         1.0,
				"buy_notional_rate":  0.0,
				"sell_notional_rate": 0.0,
			}
			rates[side+"_notional_rate"] = notional

			for name, numerator := range rates {
				value, ok := published[name]

				if !ok {
					continue
				}

				record("cvd", name, value, numerator/timeDelta, equal(value, numerator/timeDelta))
			}
		}
	}

	level3Symbols := make([]string, 0, len(level3Entries))

	for sym := range level3Entries {
		level3Symbols = append(level3Symbols, sym)
	}

	slices.Sort(level3Symbols)

	// Entries after a symbol's last trade, and on symbols without trades,
	// still replay against their venue checksums.
	for _, symbol := range level3Symbols {
		if ctx.Err() != nil {
			break
		}

		replay(symbol, math.MaxInt64)
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
			return entry.Metric.Raw()
		}
	}

	return 0
}

/*
level3Frame is one stored level3 entry: its orders, numbered consecutively from
first by the book while it held the entry's write lock.
*/
type level3Frame struct {
	first  int64
	orders []*data.Measurement
}

/*
level3Frames groups each symbol's stored level3 orders, in SeqIdx order, into
the entries ingress stored them from: a run of consecutive sequence numbers
under one checksum and type. Two adjacent entries with one checksum and type
may join one run; the checksum covers the touch, so replaying them together
yields the same book at every trade sequenced around them.
*/
func level3Frames(level3 []*data.Measurement) map[string][]level3Frame {
	bySymbol := make(map[string][]*data.Measurement)

	for _, order := range level3 {
		if order == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.truthfulness] nil level3 order in audit stream",
				nil,
			))

			continue
		}

		if order.Source == "spot:level3" {
			bySymbol[order.Label] = append(bySymbol[order.Label], order)
		}
	}

	frames := make(map[string][]level3Frame, len(bySymbol))

	for symbol, orders := range bySymbol {
		slices.SortFunc(orders, func(left, right *data.Measurement) int {
			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		for index, order := range orders {
			if index > 0 && sameLevel3Entry(orders[index-1], order) {
				last := &frames[symbol][len(frames[symbol])-1]
				last.orders = append(last.orders, order)

				continue
			}

			frames[symbol] = append(frames[symbol], level3Frame{
				first:  order.SeqIdx,
				orders: []*data.Measurement{order},
			})
		}
	}

	return frames
}

func sameLevel3Entry(previous, next *data.Measurement) bool {
	prevChecksum := data.Pull(previous.Meta("checksum"))
	nextChecksum := data.Pull(next.Meta("checksum"))
	prevType := data.Pull(previous.Meta("type"))
	nextType := data.Pull(next.Meta("type"))

	return next.SeqIdx == previous.SeqIdx+1 &&
		prevChecksum != nil && nextChecksum != nil && prevChecksum.Value == nextChecksum.Value &&
		prevType != nil && nextType != nil && prevType.Value == nextType.Value
}
