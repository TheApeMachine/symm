package audit

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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
	"trade_touch",                 // touch stamped on the trade == replayed book touch at its SeqIdx
	"response_midpoint:at",        // == (touch_bid + touch_ask) / 2 stamped on this trade
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
with the touch it stamps onto the trade, under its read lock; replaying every
stored entry sequenced before a trade through broker.Book therefore rebuilds
exactly the book that trade was stamped from. The stamped touch is checked
against that replay, cvd's response_midpoint:at against the stamped touch's
midpoint, :from against the previous frame's :at, and the log return against
ln(:at / :from).
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

	record := func(check string, observed, expected float64, ok bool) {
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
				Signal: "cvd", Metric: check, ObservedVal: observed, ExpectedVal: expected,
				Error: math.Abs(observed - expected),
			})
		}
	}

	equal := func(observed, expected float64) bool {
		return math.Abs(observed-expected) <= 1e-12+1e-9*math.Max(math.Abs(expected), math.Abs(observed))
	}

	cvdByKey := make(map[string][]*data.Measurement)
	// cvd handles a symbol's trades in order, so a trade later than the
	// symbol's last loaded cvd frame lies past the audit sample, not missing.
	lastCVD := make(map[string]int64)

	for _, m := range measurements {
		if m == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.truthfulness] nil measurement in audit stream",
				nil,
			))

			continue
		}

		if m.Source == "cvd" {
			lastCVD[m.Label] = max(lastCVD[m.Label], m.At.UnixNano())
			key := fmt.Sprintf("%s|%d", m.Label, m.At.UnixNano())
			cvdByKey[key] = append(cvdByKey[key], m)

			for entry := range m.Read() {
				if entry == nil || entry.Metric == nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"[audit.truthfulness] nil metric entry in cvd measurement",
						nil,
					), "label", m.Label)

					continue
				}

				if entry.Key == "signed_net_fraction" {
					value := entry.Metric.Raw
					record("signed_net_fraction_bounded", value, 1, value >= -1 && value <= 1)
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

	for symbol, tape := range bySymbol {
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
		symbolFrames := level3Entries[symbol]
		cursor := 0

		for _, trade := range tape {
			for cursor < len(symbolFrames) && symbolFrames[cursor].first < trade.SeqIdx {
				frame := symbolFrames[cursor]
				err := book.ApplyFrame(frame.orders)
				record("level3_replay", float64(frame.first), float64(frame.first), err == nil)
				cursor++
			}

			stampedBid, stampedAsk, stamped := stampedTouch(trade)
			replayBid, replayAsk, replayed := replayedTouch(book, symbol)

			if stamped {
				record("trade_touch", stampedBid, replayBid,
					replayed && equal(stampedBid, replayBid) && equal(stampedAsk, replayAsk))
			}

			totalTrades++
			price, qty := tradeField(trade, "price"), tradeField(trade, "qty")
			side := trade.Meta("side")
			record("trade_fields", price, qty, price > 0 && qty > 0 && (side == "buy" || side == "sell"))

			at := trade.At.UnixNano()
			prior := previous
			previous = trade
			chain := previousPublished
			previousPublished = nil

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

			previousPublished = published

			if value, ok := published["response_midpoint:at"]; ok {
				want := (stampedBid + stampedAsk) / 2.0
				record("response_midpoint:at", value, want, stamped && equal(value, want))
			}

			if value, ok := published["response_midpoint:from"]; ok {
				if previousAt, chained := chain["response_midpoint:at"]; chained {
					record("response_midpoint:from", value, previousAt, equal(value, previousAt))
				}
			}

			if value, ok := published["midpoint_log_return"]; ok {
				want := math.Log(published["response_midpoint:at"] / published["response_midpoint:from"])
				record("midpoint_log_return", value, want, equal(value, want))
			}

			if prior == nil {
				continue
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
	return next.SeqIdx == previous.SeqIdx+1 &&
		next.Meta("checksum") == previous.Meta("checksum") &&
		next.Meta("type") == previous.Meta("type")
}

/*
stampedTouch returns the touch ingress stamped onto trade, if it stamped one.
*/
func stampedTouch(trade *data.Measurement) (bid float64, ask float64, stamped bool) {
	bidEntry := data.Pull(trade.Read("touch_bid"))
	askEntry := data.Pull(trade.Read("touch_ask"))

	if bidEntry == nil || bidEntry.Metric == nil || askEntry == nil || askEntry.Metric == nil {
		return 0, 0, false
	}

	return bidEntry.Metric.Raw, askEntry.Metric.Raw, true
}

/*
replayedTouch returns the executable touch of symbol's replayed book.
*/
func replayedTouch(book *broker.Book, symbol string) (bid float64, ask float64, defined bool) {
	book.Book(symbol, func(managed *spotbook.Book) {
		bidLevel, askLevel, ok := broker.Touch(managed)

		if !ok {
			return
		}

		bid, ask, defined = bidLevel.Price.Float64(), askLevel.Price.Float64(), true
	})

	return bid, ask, defined
}
