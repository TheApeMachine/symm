package cvd

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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
				metrics = append(metrics, m.Metric.Raw)
			}

			// A symbol's first trade only seeds its volume clock: flow
			// totals wait for a closed bar, and rates, responses, and
			// baselines need an earlier trade. Nothing is defined yet.
			So(len(metrics), ShouldEqual, 0)

			for _, label := range []string{
				"trade_count", "cumulative_volume_delta", "signed_net_fraction",
				"trade_rate", "gross_notional_rate", "midpoint_log_return",
				"signed_net_fraction_baseline", "gross_notional_rate_ratio",
			} {
				entry := data.Pull(result.Read(label))
				So(entry == nil || entry.Err != nil || entry.Metric == nil, ShouldBeTrue)
			}
		})
	})
}

/*
stepSymbols hands every execution of the TestSignalStep setup its own symbols:
standardization moments live per (epoch, source, symbol, metric) for the life
of the process, so reusing a symbol would continue an earlier execution's
streams instead of starting them cold.
*/
var stepSymbols atomic.Int64

func TestSignalStep(t *testing.T) {
	Convey("Given CVD stepping a multi-leg trade tape through the real Step boundary", t, func() {
		// The futures diverge only once the streams hold enough prior
		// samples to define z-scores at the divergence.
		precursor := int(core.MinimumPrior) + 1

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
					zscores[entry.Key] = entry.Metric.Standardized
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
				values[entry.Key] = entry.Metric.Raw
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

		Convey("Every z-score is the observation against its own stream's earlier values only", func() {
			for step, zscores := range observed {
				for key, zscore := range zscores {
					// Replay the stream in its standardization space: raw,
					// ln(raw) for positive rates, or the signed log-modulus.
					space := data.CanonicalScale(key)
					var modulus core.LogModulus
					history := make([]float64, 0, step)
					current, currentDefined := 0.0, false

					// A stream only sees the steps that defined its metric.
					for _, earlier := range raws[:step+1] {
						raw, defined := earlier[key]

						if !defined {
							continue
						}

						value, inSpace := raw, true

						switch space {
						case data.ScaleLog:
							inSpace = raw > 0

							if inSpace {
								value = math.Log(raw)
							}
						case data.ScaleLogModulus:
							value, inSpace = modulus.Step(raw)
						}

						current, currentDefined = value, inSpace

						if inSpace {
							history = append(history, value)
						}
					}

					if !currentDefined {
						So(zscore, ShouldEqual, 0)
						continue
					}

					history = history[:len(history)-1]
					mean, deviation := 0.0, 0.0

					for _, value := range history {
						mean += value
					}

					mean /= float64(max(len(history), 1))

					for _, value := range history {
						deviation += (value - mean) * (value - mean)
					}

					if float64(len(history)) < core.MinimumPrior || deviation == 0 {
						So(zscore, ShouldEqual, 0)
						continue
					}

					deviation = math.Sqrt(deviation / float64(len(history)-1))

					if core.Negligible(deviation, current, mean) {
						So(zscore, ShouldEqual, 0)
						continue
					}

					want := (current - mean) / deviation
					So(zscore, ShouldAlmostEqual, want, 1e-9*math.Max(1, math.Abs(want)))
				}
			}
		})

		Convey("Standardized magnitudes are measured, so a z-score can exceed one", func() {
			largest := 0.0

			for _, zscores := range observed {
				for _, zscore := range zscores {
					largest = math.Max(largest, math.Abs(zscore))
				}
			}

			So(largest, ShouldBeGreaterThan, 1)
		})

		Convey("A different future leaves every earlier z-score unchanged", func() {
			So(observed[precursor], ShouldNotResemble, divergent[precursor])

			for step := range precursor {
				So(divergent[step], ShouldResemble, observed[step])
			}
		})
	})
}

func TestCVDUndefinedRates(t *testing.T) {
	Convey("Given CVD stepping trades with and without elapsed time", t, func() {
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		step := func(at time.Time, qty float64) map[string]float64 {
			measurement := data.NewMeasurement(
				1, "RATE/USD", "spot:trade", system.SeqIdx.Add(1), system.Tick.Add(1),
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			measurement.At = at
			measurement.From = at
			measurement.Write(
				data.NewMetric("price", 100, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			result := signal.Step(measurement)
			So(result, ShouldNotBeNil)
			values := make(map[string]float64)

			for entry := range result.Read() {
				values[entry.Key] = entry.Metric.Raw
			}

			return values
		}

		Convey("A rate needs a strictly earlier trade, never an assumed second", func() {
			first := step(origin, 1)
			same := step(origin, 2)
			later := step(origin.Add(2*time.Second), 3)

			for _, values := range []map[string]float64{first, same} {
				_, rated := values["gross_notional_rate"]
				So(rated, ShouldBeFalse)
			}

			So(later["gross_notional_rate"], ShouldAlmostEqual, 300.0/2.0, 1e-9)
			So(later["trade_rate"], ShouldAlmostEqual, 0.5, 1e-12)

			// One prior rate is a baseline but not yet a dispersion.
			_, ratio := later["gross_notional_rate_ratio"]
			_, zscore := later["gross_notional_rate_zscore"]
			So(ratio, ShouldBeFalse)
			So(zscore, ShouldBeFalse)

			after := step(origin.Add(3*time.Second), 1)
			So(after["gross_notional_rate_ratio"], ShouldAlmostEqual, 100.0/150.0, 1e-9)
			// Scored on ln(rate): the divergence is the log residual and the
			// baseline the geometric mean of the earlier rates.
			So(after["gross_notional_rate_divergence"], ShouldAlmostEqual, math.Log(100.0/150.0), 1e-12)
			So(after["gross_notional_rate_baseline"], ShouldAlmostEqual, 150.0, 1e-9)
			_, zscore = after["gross_notional_rate_zscore"]
			So(zscore, ShouldBeFalse)
		})

		Convey("An interval within the venue clock's grain has no rate or velocity", func() {
			step(origin, 1)
			step(origin.Add(2*time.Second), 3)

			// One microsecond is the venue's timestamp resolution: the stored
			// tape had such pairs, and their trade_rate was 1e6 per second.
			tick := step(origin.Add(2*time.Second+core.ClockResolution), 2)

			for _, label := range []string{
				"trade_rate", "gross_notional_rate", "net_notional_rate",
				"buy_notional_rate", "gross_notional_rate_velocity", "net_notional_rate_velocity",
			} {
				_, rated := tick[label]
				So(rated, ShouldBeFalse)
			}

			// Four grains is the shortest interval the clock resolves to
			// core.Tolerance.
			resolved := step(origin.Add(2*time.Second+5*core.ClockResolution), 2)
			So(resolved["trade_rate"], ShouldAlmostEqual, 1/(4*core.ClockResolution.Seconds()), 1e-6)
		})
	})
}
