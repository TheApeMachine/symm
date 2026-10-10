package audit

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

type tapeTrade struct {
	price, qty float64
	side       string
	at         time.Time
}

func truthTape(start time.Time) []tapeTrade {
	return []tapeTrade{
		{100, 1, "buy", start},
		{101, 2, "sell", start.Add(500 * time.Millisecond)},
		{102, 0.5, "buy", start.Add(2 * time.Second)},
	}
}

func storedTrades(tape []tapeTrade) []*data.Measurement {
	out := make([]*data.Measurement, 0, len(tape))

	for index, trade := range tape {
		m := data.NewMeasurement(1, "BTC/USD", "spot:trade", int64(index+1), int64(index+1),
			&data.StringEntry{Key: "side", Value: trade.side})
		m.At = trade.at
		m.From = trade.at
		out = append(out, m.Write(
			data.NewMetric("price", trade.price, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", trade.qty, data.UnitQuantity, data.TimescaleInstantaneous),
		))
	}

	return out
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
			values["response_midpoint:at"] = trade.price
			values["response_midpoint:from"] = prior.price
			values["midpoint_log_return"] = math.Log(trade.price / prior.price)
			values["trade_rate"] = 1 / dt
			values["buy_notional_rate"], values["sell_notional_rate"] = 0, 0

			if trade.side == "buy" {
				values["buy_notional_rate"] = trade.price * trade.qty / dt
			} else {
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

func TestTruthfulnessAudit(t *testing.T) {
	Convey("Given a stored trade tape", t, func() {
		tape := truthTape(time.Unix(1_800_000_000, 0))
		trades := storedTrades(tape)

		Convey("cvd frames that match the tape are VALID", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, nil))
			So(report.Status, ShouldEqual, VerdictValid)
			So(report.ViolationsCount, ShouldEqual, 0)
			So(report.Comparisons["trade_rate"], ShouldEqual, 2)
		})

		Convey("a rate over an invented one-second interval is a breach", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 1 {
					values["trade_rate"] = 1
				}
				return true
			}))
			So(report.Status, ShouldEqual, VerdictBreach)
			So(report.ViolationsByCheck["trade_rate"], ShouldEqual, 1)
		})

		Convey("a wrong previous price is a breach", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, func(index int, values map[string]float64) bool {
				if index == 2 {
					values["response_midpoint:from"] = 0
				}
				return true
			}))
			So(report.ViolationsByCheck["response_midpoint:from"], ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})

		Convey("a trade without its cvd frame is a breach", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, func(index int, _ map[string]float64) bool {
				return index != 1
			}))
			So(report.ViolationsByCheck["cvd_coverage"], ShouldEqual, 1)
		})

		Convey("rates on more than one of several same-time trades are a breach", func() {
			sameTime := truthTape(time.Unix(1_800_000_000, 0))
			sameTime[2].at = sameTime[1].at
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
			report := AnalyzeTruthfulness(storedTrades(sameTime), fixed)
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
			So(AnalyzeTruthfulness(storedTrades(sameTime), honest).ViolationsByCheck["rate_without_elapsed_time"], ShouldEqual, 0)
		})

		Convey("metric names no producer emits leave checks unexercised, not passed", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, func(_ int, values map[string]float64) bool {
				delete(values, "trade_rate")
				return true
			}))
			So(report.Status, ShouldEqual, VerdictInsufficient)
			So(report.UnexercisedChecks, ShouldContain, "trade_rate")
		})

		Convey("an out-of-bound signed net fraction is a breach", func() {
			report := AnalyzeTruthfulness(trades, honestCVD(tape, func(index int, values map[string]float64) bool {
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
			report := AnalyzeTruthfulness(storedTrades(bad), honestCVD(bad, nil))
			So(report.ViolationsByCheck["trade_fields"], ShouldEqual, 1)
		})
	})
}
