package cvd

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/market"
)

func TestCVDSignalMetrics(t *testing.T) {
	Convey("Given a READY CVD signal", t, func() {
		now := time.Now()
		measurement := data.NewMeasurement(
			now.UnixNano(),
			"BTC/USD",
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
			&data.StringEntry{
				Key:   "side",
				Value: "buy",
			},
		)
		measurement.At = now
		measurement.From = now
		measurement.Write(
			data.NewMetric("price", 100, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 10, data.UnitQuantity, data.TimescaleInstantaneous),
		)

		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		result := signal.Step(measurement)

		Convey("Then the result should have the expected metrics", func() {
			So(result, ShouldNotBeNil)

			metrics := make([]float64, 0)

			for m := range result.Read() {
				metrics = append(metrics, m.Metric.Raw())
			}

			// A symbol's first trade only seeds its volume clock: flow
			// totals wait for a closed bar, and midpoint responses need an earlier trade.
			So(len(metrics), ShouldEqual, 0)

			for _, label := range []string{
				"signed_net_fraction", "gross_notional", "midpoint_log_return", "flow_aligned_midpoint_return",
			} {
				entry := data.Pull(result.Read(label))
				So(entry == nil || entry.Err != nil || entry.Metric == nil, ShouldBeTrue)
			}
		})
	})
}

var stepSymbols atomic.Int64

func TestSignalStep(t *testing.T) {
	Convey("Given CVD stepping a multi-leg trade tape through the real Step boundary", t, func() {
		precursor := int(core.MinimumRankPrior) + 1

		run := stepSymbols.Add(1)
		symbol := func(name string) string {
			return fmt.Sprintf("%s%d/USD", name, run)
		}

		replay := func(tape []*data.Measurement) []map[string]float64 {
			signal := NewSignal(t.Context())
			signal.Transition(runtime.READY)
			outputs := make([]map[string]float64, 0, len(tape))

			for _, trade := range tape {
				result := signal.Step(trade)
				So(result, ShouldNotBeNil)
				So(result.Error(), ShouldBeNil)

				zscores := make(map[string]float64)

				for entry := range result.Read() {
					zscores[entry.Key] = entry.Metric.SNM()
				}

				outputs = append(outputs, zscores)
			}

			return outputs
		}

		tape := market.NewProfitableUpperTape(symbol("AAA"), 100, 0.1)
		altered := append(
			market.NewProfitableUpperTape(symbol("BBB"), 100, 0.1)[:precursor],
			market.NewFastPumpTape(symbol("BBB"), 100, 0.1, 0.15)[precursor:]...,
		)

		var raws []map[string]float64
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)

		for _, trade := range market.NewProfitableUpperTape(symbol("CCC"), 100, 0.1) {
			result := signal.Step(trade)
			values := make(map[string]float64)

			for entry := range result.Read() {
				values[entry.Key] = entry.Metric.Raw()
			}

			raws = append(raws, values)
		}

		observed := replay(tape)
		divergent := replay(altered)

		Convey("The output carries only CVD's own metrics, not the trade's price and qty", func() {
			for _, zscores := range observed {
				_, price := zscores["price"]
				_, qty := zscores["qty"]
				So(price, ShouldBeFalse)
				So(qty, ShouldBeFalse)
			}
		})

		Convey("Every SNM is bounded within [-1, 1]", func() {
			for _, zscores := range observed {
				for _, snm := range zscores {
					So(snm, ShouldBeBetweenOrEqual, -1.0, 1.0)
				}
			}
		})

		Convey("A different future leaves every earlier SNM unchanged", func() {
			So(observed[precursor], ShouldNotResemble, divergent[precursor])

			for step := range precursor {
				So(divergent[step], ShouldResemble, observed[step])
			}
		})
	})
}

func stampedTrade(symbol, side string, price, qty float64, at time.Time, bid, ask float64) *data.Measurement {
	measurement := data.NewMeasurement(
		1,
		symbol,
		"spot:trade",
		system.SeqIdx.Add(1),
		system.Tick.Add(1),
		&data.StringEntry{Key: "side", Value: side},
	)
	measurement.At = at
	measurement.From = at

	metrics := []*data.Metric{
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
	}

	if bid > 0 {
		snapshot := &broker.Snapshot{
			Depth: 1,
			Bids:  []broker.Level{{Price: decimal.NewFromFloat64(bid), Quantity: decimal.NewFromFloat64(1)}},
			Asks:  []broker.Level{{Price: decimal.NewFromFloat64(ask), Quantity: decimal.NewFromFloat64(1)}},
		}
		metrics = append(metrics, snapshot.Metrics()...)
	}

	return measurement.Write(metrics...)
}

func TestCVDSignal_BookMidpointResponse(t *testing.T) {
	Convey("Given a CVD signal stepping trades stamped with the book touch", t, func() {
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		now := time.Now()

		Convey("When trades bounce across the spread while quote midpoint does not move", func() {
			res1 := signal.Step(stampedTrade("BTC/USD", "buy", 101.0, 1.0, now, 99.0, 101.0))
			So(res1, ShouldNotBeNil)

			res2 := signal.Step(stampedTrade("BTC/USD", "sell", 99.0, 1.0, now.Add(time.Second), 99.0, 101.0))
			So(res2, ShouldNotBeNil)

			entry := data.Pull(res2.Read("midpoint_log_return"))
			So(entry, ShouldNotBeNil)
			So(entry.Metric, ShouldNotBeNil)
			So(entry.Metric.Raw(), ShouldEqual, 0.0)

			alignedEntry := data.Pull(res2.Read("flow_aligned_midpoint_return"))
			So(alignedEntry, ShouldNotBeNil)
			So(alignedEntry.Metric, ShouldNotBeNil)
			So(alignedEntry.Metric.Raw(), ShouldEqual, 0.0)
		})

		Convey("When quote midpoint moves while trade execution price is identical", func() {
			signal.Step(stampedTrade("ETH/USD", "buy", 100.0, 1.0, now, 99.0, 101.0))
			res2 := signal.Step(stampedTrade("ETH/USD", "buy", 100.0, 1.0, now.Add(time.Second), 101.0, 103.0))

			entry := data.Pull(res2.Read("midpoint_log_return"))
			So(entry, ShouldNotBeNil)
			So(entry.Metric, ShouldNotBeNil)
			So(entry.Metric.Raw(), ShouldAlmostEqual, math.Log(102.0/100.0), 1e-12)

			alignedEntry := data.Pull(res2.Read("flow_aligned_midpoint_return"))
			So(alignedEntry, ShouldNotBeNil)
			So(alignedEntry.Metric, ShouldNotBeNil)
			So(alignedEntry.Metric.Raw(), ShouldAlmostEqual, math.Log(102.0/100.0), 1e-12)
		})

		Convey("When a trade carries no touch, it has no midpoint response", func() {
			signal.Step(stampedTrade("SOL/USD", "buy", 100.0, 1.0, now, 99.0, 101.0))
			res2 := signal.Step(stampedTrade("SOL/USD", "buy", 100.0, 1.0, now.Add(time.Second), 0, 0))

			So(data.Pull(res2.Read("midpoint_log_return")), ShouldBeNil)
			So(data.Pull(res2.Read("flow_aligned_midpoint_return")), ShouldBeNil)
		})
	})
}
