package hawkes

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/trade"
)

func TestHawkesSymbolFitForEvents(t *testing.T) {
	now, stream := fitForEventsFixture(t)

	convey.Convey("Given a fitted Hawkes symbol", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())

		first, ok := symbol.fitForEvents(stream, now)

		convey.So(ok, convey.ShouldBeTrue)
		convey.So(first.MuBuy, convey.ShouldBeGreaterThan, 0)

		firstMuBuy := symbol.fit.MuBuy
		firstAlphaBB := symbol.fit.AlphaBB
		ratioCount := len(symbol.intensityRatios)

		convey.Convey("When the event window is unchanged", func() {
			second, ok := symbol.fitForEvents(stream, now.Add(time.Millisecond))

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(symbol.fit.MuBuy, convey.ShouldEqual, firstMuBuy)
			convey.So(symbol.fit.AlphaBB, convey.ShouldEqual, firstAlphaBB)
			convey.So(len(symbol.intensityRatios), convey.ShouldEqual, ratioCount)
			convey.So(second.BuyIntensity, convey.ShouldNotEqual, first.BuyIntensity)
		})

		convey.Convey("When the measurement horizon advances", func() {
			second, ok := symbol.fitForEvents(stream, now.Add(100*time.Millisecond))

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(second.MuBuy, convey.ShouldEqual, firstMuBuy)
			convey.So(second.AlphaBB, convey.ShouldEqual, firstAlphaBB)
			convey.So(second.BuyIntensity, convey.ShouldNotEqual, first.BuyIntensity)
		})

		convey.Convey("When a new trade enters the window", func() {
			ticks := ticksFromSideEvents(stream.BuyTimes(), stream.SellTimes())
			ticks = append(ticks, trade.Data{
				Side:      "buy",
				Timestamp: now.Add(-time.Second),
			})
			_, updatedStream, ok := FitContextFromTicks(ticks, time.Time{}, now)

			convey.So(ok, convey.ShouldBeTrue)

			_, ok = symbol.fitForEvents(updatedStream, now.Add(symbol.fitCooldown))

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(symbol.intensityRatios), convey.ShouldBeGreaterThan, ratioCount)
		})

		convey.Convey("When the event window changes inside the fit cooldown", func() {
			ticks := ticksFromSideEvents(stream.BuyTimes(), stream.SellTimes())
			ticks = append(ticks, trade.Data{
				Side:      "buy",
				Timestamp: now.Add(-time.Second),
			})
			_, updatedStream, ok := FitContextFromTicks(ticks, time.Time{}, now)

			convey.So(ok, convey.ShouldBeTrue)

			second, ok := symbol.fitForEvents(updatedStream, now.Add(time.Millisecond))

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(symbol.fit.MuBuy, convey.ShouldEqual, firstMuBuy)
			convey.So(symbol.fit.AlphaBB, convey.ShouldEqual, firstAlphaBB)
			convey.So(len(symbol.intensityRatios), convey.ShouldEqual, ratioCount)
			convey.So(second.BuyIntensity, convey.ShouldNotEqual, first.BuyIntensity)
		})
	})
}

func TestHawkesSymbolApplyFeedback(t *testing.T) {
	now, stream := fitForEventsFixture(t)

	convey.Convey("Given a cached Hawkes fit", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())

		_, ok := symbol.fitForEvents(stream, now)

		convey.So(ok, convey.ShouldBeTrue)

		ratioCount := len(symbol.intensityRatios)

		_, ok = symbol.fitForEvents(stream, now)

		convey.So(ok, convey.ShouldBeTrue)
		convey.So(len(symbol.intensityRatios), convey.ShouldEqual, ratioCount)

		convey.Convey("When prediction feedback arrives", func() {
			symbol.ApplyFeedback(engine.PredictionFeedback{
				Source:          hawkesSource,
				Symbol:          "BTC/EUR",
				PredictedReturn: 0.1,
				ActualReturn:    0.05,
			})

			convey.Convey("It should invalidate the fit cache", func() {
				_, ok := symbol.fitForEvents(stream, now)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(len(symbol.intensityRatios), convey.ShouldBeGreaterThan, ratioCount)
			})

			convey.Convey("It should lower excitation calibration in the prior", func() {
				prior := symbol.fit.Calibrated(symbol.calibrator.Scale())

				convey.So(symbol.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
				convey.So(prior.AlphaBB, convey.ShouldAlmostEqual, symbol.fit.AlphaBB*0.5, 0.0001)
			})
		})
	})
}

func TestHawkesSymbolGaugeScore(t *testing.T) {
	convey.Convey("Given a symbol with calibration history", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())
		symbol.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

		convey.Convey("It should score without persisting history when persist is false", func() {
			historyLen := len(symbol.confidenceHistory)
			score := symbol.gaugeScore(2.5, false)

			convey.So(score, convey.ShouldBeGreaterThan, 0)
			convey.So(score, convey.ShouldBeLessThanOrEqualTo, 1)
			convey.So(len(symbol.confidenceHistory), convey.ShouldEqual, historyLen)
		})

		convey.Convey("It should append history when persist is true", func() {
			historyLen := len(symbol.confidenceHistory)
			score := symbol.gaugeScore(2.5, true)

			convey.So(score, convey.ShouldBeGreaterThan, 0)
			convey.So(score, convey.ShouldBeLessThanOrEqualTo, 1)
			convey.So(len(symbol.confidenceHistory), convey.ShouldEqual, historyLen+1)
		})
	})

	convey.Convey("Given a cold symbol", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())

		convey.Convey("It should reject confidence before calibration history exists", func() {
			convey.So(symbol.gaugeScore(3.2, true), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given a warmed symbol and mid-fence confidence", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())
		symbol.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4, 2.6, 2.8, 3.0, 3.2}
		fence := engine.ConfidenceFence(symbol.confidenceHistory)

		convey.Convey("It should normalize inside the unit interval", func() {
			score := symbol.calibrator.NormalizeConfidence(fence/2, symbol.confidenceHistory)

			convey.So(score, convey.ShouldBeGreaterThan, 0)
			convey.So(score, convey.ShouldBeLessThan, 1)
		})
	})

	convey.Convey("Given a non-positive raw score", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())

		convey.Convey("It should return zero", func() {
			convey.So(symbol.gaugeScore(0, true), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given minimum calibration history", t, func() {
		symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())
		minHistory := confidenceHistoryCap(bivariateParamCount * 2)

		for index := 0; index < minHistory; index++ {
			symbol.confidenceHistory = append(symbol.confidenceHistory, 1.2)
		}

		convey.Convey("It should return unit-scale scores", func() {
			first := symbol.gaugeScore(2.5, true)
			second := symbol.gaugeScore(1.1, true)

			convey.So(first, convey.ShouldBeGreaterThan, 0)
			convey.So(first, convey.ShouldBeLessThanOrEqualTo, 1)
			convey.So(second, convey.ShouldBeGreaterThan, 0)
			convey.So(second, convey.ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func BenchmarkHawkesSymbolFitForEvents(b *testing.B) {
	start := time.Unix(10_000, 0)
	buyEvents := burstEvents(start, 16, 50*time.Millisecond)
	sellEvents := burstEvents(start.Add(5*time.Millisecond), 6, 80*time.Millisecond)
	now := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	ticks := ticksFromSideEvents(buyEvents, sellEvents)
	_, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok {
		b.Fatal("expected fit context from fixture")
	}

	symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())

	if _, ok := symbol.fitForEvents(stream, now); !ok {
		b.Fatal("expected warm fit")
	}

	horizon := now.Add(time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, ok := symbol.fitForEvents(stream, horizon); !ok {
			b.Fatal("expected cached fit")
		}
	}
}
