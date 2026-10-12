package audit

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
tapeTrade is one stored trade and the executable touch the book held at its
sequence number. The touch's midpoint lies inside the spread, never on the
trade price.
*/
type tapeTrade struct {
	price, qty float64
	side       string
	at         time.Time
	bid, ask   float64
}

func (trade tapeTrade) midpoint() float64 {
	return (trade.bid + trade.ask) / 2
}

func truthTape(start time.Time) []tapeTrade {
	return []tapeTrade{
		{100, 1, "buy", start, 99.9, 100},
		{101, 2, "sell", start.Add(500 * time.Millisecond), 101, 101.1},
		{102, 0.5, "buy", start.Add(2 * time.Second), 101.8, 102},
	}
}

/*
storedTape is what ingress stores for the tape: before each trade, the level3
entry that moves the book to that trade's touch (a snapshot first, then an
update replacing both touch orders), numbered as the book numbers an applied
entry; then the trade, stamped with the live book's snapshot through
broker.Book.Stamp exactly as ingress stamps it. Checksum zero leaves the venue
checksum unverified, since these books are not venue books.
*/
func storedTape(t *testing.T, tape []tapeTrade) (trades []*data.Measurement, level3 []*data.Measurement) {
	sequence := int64(0)
	live := broker.NewBook(t.Context(), spot.NewNormalizer())

	order := func(entryType, side, id, event string, price float64, at time.Time) *data.Measurement {
		sequence++
		measurement := data.NewMeasurement(1, "BTC/USD", "spot:level3", sequence, sequence,
			&data.StringEntry{Key: "type", Value: entryType},
			&data.StringEntry{Key: "order_id", Value: id},
			&data.StringEntry{Key: "side", Value: side},
			&data.StringEntry{Key: "event", Value: event},
			&data.StringEntry{Key: "checksum", Value: "0"},
		)
		measurement.At = at
		measurement.From = at

		return measurement.Write(
			data.NewExactMetric("limit_price", decimal.NewFromFloat64(price), data.UnitPrice, data.TimescaleInstantaneous),
			data.NewExactMetric("order_qty", decimal.NewFromFloat64(1), data.UnitQuantity, data.TimescaleInstantaneous),
		)
	}

	for index, trade := range tape {
		bidID, askID := "B"+string(rune('0'+index)), "A"+string(rune('0'+index))
		var entry []*data.Measurement

		if index == 0 {
			entry = []*data.Measurement{
				order("snapshot", "bid", bidID, "add", trade.bid, trade.at),
				order("snapshot", "ask", askID, "add", trade.ask, trade.at),
			}
		}

		if index > 0 {
			prior := tape[index-1]
			entry = []*data.Measurement{
				order("update", "bid", "B"+string(rune('0'+index-1)), "delete", prior.bid, trade.at),
				order("update", "bid", bidID, "add", trade.bid, trade.at),
				order("update", "ask", "A"+string(rune('0'+index-1)), "delete", prior.ask, trade.at),
				order("update", "ask", askID, "add", trade.ask, trade.at),
			}
		}

		So(live.ApplyFrame(entry), ShouldBeNil)
		level3 = append(level3, entry...)
		_, snapshot := live.Stamp("BTC/USD")
		So(snapshot, ShouldNotBeNil)

		sequence++
		measurement := data.NewMeasurement(1, "BTC/USD", "spot:trade", sequence, sequence,
			&data.StringEntry{Key: "side", Value: trade.side})
		measurement.At = trade.at
		measurement.From = trade.at
		trades = append(trades, measurement.Write(append([]*data.Metric{
			data.NewMetric("price", trade.price, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", trade.qty, data.UnitQuantity, data.TimescaleInstantaneous),
		}, snapshot.Metrics()...)...))
	}

	return trades, level3
}

/*
honestCVD is what cvd must publish for the tape; corrupt rewrites one value
of one frame (or drops the frame) to make a known-bad fixture.
*/
func honestCVD(tape []tapeTrade, corrupt func(index int, values map[string]float64) bool) []*data.Measurement {
	out := make([]*data.Measurement, 0, len(tape))

	for index, trade := range tape {
		values := map[string]float64{"signed_net_fraction": 0.5}

		if index > 0 {
			prior := tape[index-1]
			dt := trade.at.Sub(prior.at).Seconds()
			values["response_midpoint:at"] = trade.midpoint()
			values["response_midpoint:from"] = prior.midpoint()
			values["midpoint_log_return"] = math.Log(trade.midpoint() / prior.midpoint())
			values["trade_rate"] = 1 / dt
			values["buy_notional_rate"], values["sell_notional_rate"] = 0, 0

			if trade.side == "buy" {
				values["buy_notional_rate"] = trade.price * trade.qty / dt
			}

			if trade.side == "sell" {
				values["sell_notional_rate"] = trade.price * trade.qty / dt
			}
		}

		if corrupt != nil && !corrupt(index, values) {
			continue
		}

		m := data.NewMeasurement(1, "BTC/USD", "cvd", int64(100+index), int64(100+index))
		m.At = trade.at
		m.From = trade.at
		metrics := make([]*data.Metric, 0, len(values))

		for key, value := range values {
			metrics = append(metrics, data.NewMetric(key, value, data.UnitDimensionless, data.TimescaleTick))
		}

		out = append(out, m.Write(metrics...))
	}

	return out
}

func TestAnalyzeTruthfulness(t *testing.T) {
	Convey("Given a stored trade tape and the level3 tape its books came from", t, func() {
		tape := truthTape(time.Unix(1_800_000_000, 0))
		trades, level3 := storedTape(t, tape)

		Convey("cvd frames that match the tape are VALID", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, nil))
			So(report.Status, ShouldEqual, VerdictValid)
			So(report.ViolationsCount, ShouldEqual, 0)
			So(report.Comparisons["level3_replay"], ShouldEqual, 3)
			So(report.Comparisons["trade_book"], ShouldEqual, 3)
			So(report.Comparisons["trade_rate"], ShouldEqual, 2)
			So(report.Comparisons["response_midpoint:at"], ShouldEqual, 2)
			So(report.Comparisons["response_midpoint:from"], ShouldEqual, 1)
			So(report.Comparisons["midpoint_log_return"], ShouldEqual, 2)
		})

		Convey("a book the stored level3 tape does not rebuild is a breach", func() {
			stamped := truthTape(time.Unix(1_800_000_000, 0))
			stamped[1].bid = 100.5
			stampedTrades, _ := storedTape(t, stamped)

			report := AnalyzeTruthfulness(t.Context(), stampedTrades, level3, honestCVD(stamped, nil))
			So(report.ViolationsByCheck["trade_book"], ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a stamped book that cannot be decoded is a breach", func() {
			broken := data.NewMeasurement(1, "BTC/USD", "spot:trade", trades[0].SeqIdx, trades[0].Tick,
				&data.StringEntry{Key: "side", Value: "buy"})
			broken.At, broken.From = trades[0].At, trades[0].At
			broken.Write(
				data.NewMetric("price", tape[0].price, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", tape[0].qty, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewExactMetric("bid_price_0", decimal.NewFromFloat64(tape[0].bid), data.UnitPrice, data.TimescaleInstantaneous),
			)

			report := AnalyzeTruthfulness(t.Context(), append([]*data.Measurement{broken}, trades[1:]...), level3, honestCVD(tape, nil))
			So(report.ViolationsByCheck["trade_book"], ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a response priced on the trade instead of the book midpoint is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 1 {
					values["response_midpoint:at"] = tape[1].price
				}
				return true
			}))
			So(report.ViolationsByCheck["response_midpoint:at"], ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a rate over an invented one-second interval is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 1 {
					values["trade_rate"] = 1
				}
				return true
			}))
			So(report.Status, ShouldEqual, VerdictBreach)
			So(report.ViolationsByCheck["trade_rate"], ShouldEqual, 1)
		})

		Convey("a response that does not start at the previous frame's midpoint is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 2 {
					// A stale prior midpoint, internally consistent with its return.
					values["response_midpoint:from"] = tape[0].midpoint()
					values["midpoint_log_return"] = math.Log(tape[2].midpoint() / tape[0].midpoint())
				}
				return true
			}))
			So(report.ViolationsByCheck["response_midpoint:from"], ShouldEqual, 1)
			So(report.ViolationsByCheck["midpoint_log_return"], ShouldEqual, 0)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a log return that is not ln(at / from) is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 1 {
					values["midpoint_log_return"] = math.Log(tape[1].price / tape[0].price)
				}
				return true
			}))
			So(report.ViolationsByCheck["midpoint_log_return"], ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a trade without its cvd frame is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, _ map[string]float64) bool {
				return index != 1
			}))
			So(report.ViolationsByCheck["cvd_coverage"], ShouldEqual, 1)
		})

		Convey("rates on more than one of several same-time trades are a breach", func() {
			sameTime := truthTape(time.Unix(1_800_000_000, 0))
			sameTime[2].at = sameTime[1].at
			sameTrades, sameLevel3 := storedTape(t, sameTime)
			frames := honestCVD(sameTime, nil)
			// honestCVD priced the third trade over a zero interval (+Inf);
			// an invented interval makes it finite, as a dt default would.
			fixed := honestCVD(sameTime, func(index int, values map[string]float64) bool {
				if index == 2 {
					values["trade_rate"] = 1
				}
				return true
			})
			So(len(frames), ShouldEqual, 3)
			report := AnalyzeTruthfulness(t.Context(), sameTrades, sameLevel3, fixed)
			So(report.AmbiguousTrades, ShouldEqual, 2)
			So(report.ViolationsByCheck["rate_without_elapsed_time"], ShouldEqual, 1)

			honest := honestCVD(sameTime, func(index int, values map[string]float64) bool {
				if index == 2 {
					delete(values, "trade_rate")
					delete(values, "buy_notional_rate")
					delete(values, "sell_notional_rate")
				}
				return true
			})
			So(AnalyzeTruthfulness(t.Context(), sameTrades, sameLevel3, honest).ViolationsByCheck["rate_without_elapsed_time"], ShouldEqual, 0)
		})

		Convey("metric names no producer emits leave checks unexercised, not passed", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(_ int, values map[string]float64) bool {
				delete(values, "trade_rate")
				return true
			}))
			So(report.Status, ShouldEqual, VerdictInsufficient)
			So(report.UnexercisedChecks, ShouldContain, "trade_rate")
		})

		Convey("an out-of-bound signed net fraction is a breach", func() {
			report := AnalyzeTruthfulness(t.Context(), trades, level3, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 0 {
					values["signed_net_fraction"] = 2.5
				}
				return true
			}))
			So(report.ViolationsByCheck["signed_net_fraction_bounded"], ShouldEqual, 1)
		})

		Convey("an invalid trade is a breach", func() {
			bad := truthTape(time.Unix(1_800_000_000, 0))
			bad[0].side = "invalid_side"
			badTrades, badLevel3 := storedTape(t, bad)
			report := AnalyzeTruthfulness(t.Context(), badTrades, badLevel3, honestCVD(bad, nil))
			So(report.ViolationsByCheck["trade_fields"], ShouldEqual, 1)
		})
	})
}
