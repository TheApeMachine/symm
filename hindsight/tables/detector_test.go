package tables_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestStreamingDetector(t *testing.T) {
	Convey("Given a StreamingDetector with $200 initial balance", t, func() {
		var completed []tables.ExcursionRecord
		epoch := int64(1000)

		detector := tables.NewStreamingDetector(epoch, 200.0, func(record tables.ExcursionRecord) {
			completed = append(completed, record)
		})

		Convey("When an upward price move clears friction and reverses", func() {
			at := time.Now().UTC()
			symbol := "BTC/USD"

			// Baseline precursor ticks
			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 50000},
						"ask":  {Raw: 50010},
						"last": {Raw: 50005},
					},
				})
			}

			// Price rises significantly (+5%), clearing friction
			for tick := int64(11); tick <= 20; tick++ {
				price := 50000.0 + float64(tick-10)*250.0
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 5},
						"ask":  {Raw: price + 5},
						"last": {Raw: price},
					},
				})
			}

			// Peak reached at 52500, then retraces by 40% of the gain
			for tick := int64(21); tick <= 45; tick++ {
				price := 52500.0 - float64(tick-20)*70.0
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 5},
						"ask":  {Raw: price + 5},
						"last": {Raw: price},
					},
				})
			}

			So(len(completed), ShouldBeGreaterThanOrEqualTo, 1)
			first := completed[0]
			So(first.Symbol, ShouldEqual, symbol)
			So(first.Direction, ShouldEqual, "upward")
			So(first.ClearsFriction, ShouldBeTrue)
			So(first.Profit, ShouldBeGreaterThan, 0)
			So(first.PositionSize, ShouldAlmostEqual, 40.0, 1.0) // 20% of 200
			So(first.AnchorTick, ShouldBeGreaterThanOrEqualTo, 10)
			So(first.ExtremumTick, ShouldBeGreaterThanOrEqualTo, first.AnchorTick)
			So(first.ExitTick, ShouldBeGreaterThanOrEqualTo, first.ExtremumTick)
			So(first.PostEndTick, ShouldBeGreaterThanOrEqualTo, first.ExitTick)
		})

		Convey("When a downward price drop occurs", func() {
			at := time.Now().UTC()
			symbol := "ETH/USD"

			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 3000},
						"ask":  {Raw: 3001},
						"last": {Raw: 3000.5},
					},
				})
			}

			for tick := int64(11); tick <= 20; tick++ {
				price := 3000.0 - float64(tick-10)*20.0
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 1},
						"ask":  {Raw: price + 1},
						"last": {Raw: price},
					},
				})
			}

			// Bounce back triggers exit
			for tick := int64(21); tick <= 45; tick++ {
				price := 2800.0 + float64(tick-20)*10.0
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 1},
						"ask":  {Raw: price + 1},
						"last": {Raw: price},
					},
				})
			}

			So(len(completed), ShouldBeGreaterThanOrEqualTo, 1)
			excursion := completed[0]
			So(excursion.Symbol, ShouldEqual, symbol)
			So(excursion.Direction, ShouldEqual, "downward")
			So(excursion.ClearsFriction, ShouldBeFalse)
		})

		Convey("When concurrent opportunities occur on two symbols", func() {
			at := time.Now().UTC()

			// Baseline
			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "SOL/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 100},
						"ask":  {Raw: 100.1},
						"last": {Raw: 100.05},
					},
				})
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "AVAX/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 50},
						"ask":  {Raw: 50.05},
						"last": {Raw: 50.02},
					},
				})
			}

			// SOL initiates first: takes 20% of 200 = ~40
			for tick := int64(11); tick <= 15; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "SOL/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 100.0 + float64(tick-10)*1.0},
						"ask":  {Raw: 100.1 + float64(tick-10)*1.0},
						"last": {Raw: 100.05 + float64(tick-10)*1.0},
					},
				})
			}

			// AVAX initiates second: remaining balance is ~160, so 20% of 160 = ~32
			for tick := int64(16); tick <= 20; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "AVAX/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 50.0 + float64(tick-15)*0.8},
						"ask":  {Raw: 50.05 + float64(tick-15)*0.8},
						"last": {Raw: 50.02 + float64(tick-15)*0.8},
					},
				})
			}

			// Exit both
			for tick := int64(21); tick <= 45; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "SOL/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 103.0},
						"ask":  {Raw: 103.1},
						"last": {Raw: 103.05},
					},
				})
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  "AVAX/USD",
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 52.0},
						"ask":  {Raw: 52.05},
						"last": {Raw: 52.02},
					},
				})
			}

			So(len(completed), ShouldBeGreaterThanOrEqualTo, 2)
			var solRec, avaxRec tables.ExcursionRecord
			for _, rec := range completed {
				if rec.Symbol == "SOL/USD" {
					solRec = rec
				}
				if rec.Symbol == "AVAX/USD" {
					avaxRec = rec
				}
			}

			So(solRec.PositionSize, ShouldAlmostEqual, 40.0, 1.0)
			So(avaxRec.PositionSize, ShouldAlmostEqual, 32.0, 1.0)
		})

		Convey("When an upward move fails to clear friction (subfriction)", func() {
			at := time.Now().UTC()
			symbol := "DOGE/USD"

			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 0.1000},
						"ask":  {Raw: 0.1001},
						"last": {Raw: 0.10005},
					},
				})
			}

			// Breakout at tick 11 initiates upward excursion
			for tick := int64(11); tick <= 18; tick++ {
				price := 0.10040 + float64(tick-11)*0.00003
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 0.00005},
						"ask":  {Raw: price + 0.00005},
						"last": {Raw: price},
					},
				})
			}

			// Rollover and retracement back down
			for tick := int64(19); tick <= 45; tick++ {
				price := 0.10061 - float64(tick-18)*0.00002
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 0.00005},
						"ask":  {Raw: price + 0.00005},
						"last": {Raw: price},
					},
				})
			}

			var dogeRec *tables.ExcursionRecord
			for _, rec := range completed {
				if rec.Symbol == symbol {
					recCopy := rec
					dogeRec = &recCopy
					break
				}
			}

			So(dogeRec, ShouldNotBeNil)
			So(dogeRec.Direction, ShouldEqual, "upward")
			So(dogeRec.ClearsFriction, ShouldBeFalse)
			So(dogeRec.Status, ShouldEqual, "subfriction")
			So(dogeRec.Profit, ShouldBeLessThan, 0)
		})

		Convey("When tape is choppy with oscillating direction inside noise band", func() {
			at := time.Now().UTC()
			symbol := "DOT/USD"

			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 5.000},
						"ask":  {Raw: 5.002},
						"last": {Raw: 5.001},
					},
				})
			}

			// Oscillate up and down repeatedly across anchor 5.001 within noise band (±0.04%)
			for tick := int64(11); tick <= 150; tick++ {
				offset := 0.002
				if tick%2 == 0 {
					offset = -0.002
				}
				price := 5.001 + offset
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: price - 0.001},
						"ask":  {Raw: price + 0.001},
						"last": {Raw: price},
					},
				})
			}

			var dotRec *tables.ExcursionRecord
			for _, rec := range completed {
				if rec.Symbol == symbol {
					recCopy := rec
					dotRec = &recCopy
					break
				}
			}

			So(dotRec, ShouldNotBeNil)
			So(dotRec.Direction, ShouldEqual, "choppy")
			So(dotRec.ClearsFriction, ShouldBeFalse)
			So(dotRec.Status, ShouldEqual, "choppy")
		})

		Convey("When tape is flat with prolonged quietness", func() {
			at := time.Now().UTC()
			symbol := "ADA/USD"

			for tick := int64(1); tick <= 10; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 0.3500},
						"ask":  {Raw: 0.3501},
						"last": {Raw: 0.35005},
					},
				})
			}

			// Dead flat ticks with no oscillation for >128 ticks
			for tick := int64(11); tick <= 140; tick++ {
				detector.Process(&data.Measurement[float64]{
					Source: "spot_ticker",
					Label:  symbol,
					SeqIdx: tick,
					At:     at,
					Metrics: map[string]data.Metric[float64]{
						"bid":  {Raw: 0.3500},
						"ask":  {Raw: 0.3501},
						"last": {Raw: 0.35005},
					},
				})
			}

			var adaRec *tables.ExcursionRecord
			for _, rec := range completed {
				if rec.Symbol == symbol {
					recCopy := rec
					adaRec = &recCopy
					break
				}
			}

			So(adaRec, ShouldNotBeNil)
			So(adaRec.Direction, ShouldEqual, "flat")
			So(adaRec.ClearsFriction, ShouldBeFalse)
			So(adaRec.Status, ShouldEqual, "flat")
		})
	})
}
