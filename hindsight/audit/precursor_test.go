package audit

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestPrecursorSeparation(t *testing.T) {
	Convey("Given the Precursor Separation audit engine (Stage 5)", t, func() {
		ctx := context.Background()

		Convey("When invoked with invalid or non-positive taker fee", func() {
			result := AnalyzePrecursorSeparation(
				ctx, nil, 1, "BTC/USD", store.NewGrid(), nil, nil, 10, nil, 0.0,
			)
			So(result.Passed, ShouldBeFalse)
			So(result.SummaryText, ShouldContainSubstring, "strictly positive")
		})

		Convey("When invoked with nil catalog or grid", func() {
			result := AnalyzePrecursorSeparation(
				ctx, nil, 1, "BTC/USD", nil, nil, nil, 10, nil, 0.0026,
			)
			So(result.Passed, ShouldBeFalse)
			So(result.SummaryText, ShouldContainSubstring, "Grid unavailable")
		})

		Convey("When computing Jensen-Shannon Divergence", func() {
			distA := map[string]int{"R01": 50, "R02": 50}
			distB := map[string]int{"R01": 50, "R02": 50}
			jsdIdentical := computeJSD(distA, distB)
			So(jsdIdentical, ShouldAlmostEqual, 0.0, 1e-6)

			distC := map[string]int{"R03": 50, "R04": 50}
			jsdDisjoint := computeJSD(distA, distC)
			So(jsdDisjoint, ShouldAlmostEqual, 1.0, 1e-4)

			jsdEmpty := computeJSD(nil, distA)
			So(jsdEmpty, ShouldEqual, 0.0)
		})

		Convey("When evaluating predictive skill with discriminating precursor tokens", func() {
			eventTokens := map[string]int{
				"R01": 40,
				"R02": 10,
			}
			controlTokens := map[string]int{
				"R01": 5,
				"R02": 45,
			}

			skill := evaluatePredictiveSkill(eventTokens, controlTokens)
			So(skill.Status, ShouldEqual, "MEASURED")
			So(skill.EvaluatedSamples, ShouldEqual, 100)
			So(skill.BalancedAccuracy, ShouldBeGreaterThan, 0.70)
			So(skill.MCC, ShouldBeGreaterThan, 0.50)
			So(skill.PredictiveGainBits, ShouldBeGreaterThan, 0.20)
			So(skill.TopPrecursorTokens, ShouldContain, "R01")
			So(skill.Passed, ShouldBeTrue)
		})

		Convey("When evaluating predictive skill with insufficient samples", func() {
			eventTokens := map[string]int{"R01": 2}
			controlTokens := map[string]int{"R02": 2}
			skill := evaluatePredictiveSkill(eventTokens, controlTokens)
			So(skill.Status, ShouldEqual, "INSUFFICIENT_DATA")
			So(skill.Passed, ShouldBeFalse)
		})

		Convey("When evaluating economic relevance (friction clearance)", func() {
			takerFee := 0.0026 // 26 bps taker fee, 52 bps roundtrip

			excursions := []excursionInterval{
				{
					symbol: "BTC/USD",
					class:  "up",
					bPrice: 50000.0,
					cPrice: 51000.0, // +2.0% gross > 0.52% fee -> profitable
				},
				{
					symbol: "BTC/USD",
					class:  "up",
					bPrice: 50000.0,
					cPrice: 50500.0, // +1.0% gross > 0.52% fee -> profitable
				},
				{
					symbol: "BTC/USD",
					class:  "up_friction",
					bPrice: 50000.0,
					cPrice: 50100.0, // +0.2% gross < 0.52% fee -> unprofitable
				},
			}

			economic := evaluateEconomicRelevance(excursions, takerFee)
			So(economic.Status, ShouldEqual, "MEASURED")
			So(economic.EvaluatedExcursions, ShouldEqual, 3)
			So(economic.ProfitableExcursions, ShouldEqual, 2)
			So(economic.UnprofitableExcursions, ShouldEqual, 1)
			So(economic.FrictionClearanceRate, ShouldAlmostEqual, 2.0/3.0, 1e-4)
			So(economic.GrossMeanReturn, ShouldBeGreaterThan, economic.RoundTripFeeRate)
			So(economic.NetMeanReturn, ShouldBeGreaterThan, 0.0)
			So(economic.Passed, ShouldBeTrue)
		})

		Convey("When evaluating end-to-end precursor separation with synthetic detections", func() {
			grid := store.NewGrid()
			now := time.Now()

			det1 := data.NewMeasurement(1, "BTC/USD", "detector", 1, 100, &data.StringEntry{Key: "type", Value: "up"})
			det1.At = now
			det1.From = now.Add(-time.Minute)
			det1 = det1.Write(
				data.NewMetric("start_tick", 10, data.UnitCount, data.TimescaleTick),
				data.NewMetric("b_tick", 30, data.UnitCount, data.TimescaleTick),
				data.NewMetric("c_tick", 60, data.UnitCount, data.TimescaleTick),
				data.NewExactMetric("b_price", decimal.NewFromFloat64(50000), data.UnitPrice, data.TimescaleTick),
				data.NewExactMetric("c_price", decimal.NewFromFloat64(51000), data.UnitPrice, data.TimescaleTick),
			)

			det2 := data.NewMeasurement(1, "BTC/USD", "detector", 2, 200, &data.StringEntry{Key: "type", Value: "down"})
			det2.At = now
			det2.From = now.Add(-time.Minute)
			det2 = det2.Write(
				data.NewMetric("start_tick", 110, data.UnitCount, data.TimescaleTick),
				data.NewMetric("b_tick", 130, data.UnitCount, data.TimescaleTick),
				data.NewMetric("c_tick", 160, data.UnitCount, data.TimescaleTick),
				data.NewExactMetric("b_price", decimal.NewFromFloat64(50000), data.UnitPrice, data.TimescaleTick),
				data.NewExactMetric("c_price", decimal.NewFromFloat64(49000), data.UnitPrice, data.TimescaleTick),
			)

			detections := []*data.Measurement{det1, det2}
			ticks := []int64{10, 20, 30, 40, 50, 60, 110, 120, 130, 140, 150, 160}
			tickMeasurements := make(map[int64][]*data.Measurement)

			for _, tick := range ticks {
				meas := data.NewMeasurement(1, "BTC/USD", "sensor", tick, tick)
				meas.At = now.Add(time.Duration(tick) * time.Second)
				meas.From = meas.At
				meas = meas.Write(
					data.NewMetric("midpoint", 50000+float64(tick)*5, data.UnitPrice, data.TimescaleTick),
				)
				tickMeasurements[tick] = []*data.Measurement{meas}
			}

			result := AnalyzePrecursorSeparation(
				ctx, nil, 1, "BTC/USD", grid, ticks, tickMeasurements, 20, detections, 0.0026,
			)

			So(result.DetectionsFound, ShouldEqual, 2)
			So(result.EconomicRelevance.Status, ShouldEqual, "MEASURED")
			So(result.EconomicRelevance.EvaluatedExcursions, ShouldEqual, 1)
			So(result.EconomicRelevance.FrictionClearanceRate, ShouldEqual, 1.0)
			So(result.SummaryText, ShouldContainSubstring, "Precursor: 2 detections")
		})
	})
}
