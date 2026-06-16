package prediction

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func init() {
	viper.Set("story.prediction.interval", 0)
	viper.Set("signals.prediction.measurements_capacity", 16)
}

func newTestSignal(testingTB testing.TB) *Signal {
	testingTB.Helper()

	signal := NewSignal(context.Background(), nil)
	testingTB.Cleanup(func() {
		_ = signal.Close()
	})

	return signal
}

func measurementArtifact(scope string) *datura.Artifact {
	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope(scope)
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	updates := make(krakenmarket.TradeUpdates, count)

	for index := range count {
		updates[index] = &krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	signal.trade.Update(updates)
}

func TestSignalRecordFeatureMeasurement(testingTB *testing.T) {
	Convey("Given upstream measurements", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.75,
		})

		Convey("It should store the feature snapshot", func() {
			So(state.features[featureSourceIndex(logic.SourcePumpDump)], ShouldEqual, 0.75)
		})
	})
}

func TestSignalRecord(testingTB *testing.T) {
	Convey("Given a new signal", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		So(state.recordTrade(), ShouldBeTrue)
		So(state.recordTrade(), ShouldBeTrue)

		Convey("It should count down warmup", func() {
			capacity := measurementsCapacity()

			So(state.warmupRemaining, ShouldEqual, capacity-2)
			So(state.warmupFilled(), ShouldEqual, 2)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given source confidences and trade prices", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.8,
		})
		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceHawkes,
			Symbol:     "ETH/EUR",
			Confidence: 0.2,
		})

		state.realizedMagnitudeEMA = 0.01
		coefficients := state.learner.Coefficients()
		coefficients[featureSourceIndex(logic.SourcePumpDump)+1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		measurement, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should publish a unit-band forward confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePrediction)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
			So(measurement.Elapsed, ShouldEqual, signal.horizon.Seconds())
		})
	})

	Convey("Given the configured forecast interval", testingTB, func() {
		viper.Set("story.prediction.interval", time.Second)

		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")
		signal.forecastInterval = time.Second

		state.features[0] = 0.8
		state.features[1] = 0.2
		state.realizedMagnitudeEMA = 0.01
		coefficients := state.learner.Coefficients()
		coefficients[1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)
		series := signal.trade.Series("ETH/EUR")

		first, firstErr := state.fromSeries(signal, "ETH/EUR", series.Prices, series.Volumes, nil, true, eventAt.Add(time.Second))
		second, secondErr := state.fromSeries(signal, "ETH/EUR", series.Prices, series.Volumes, nil, true, eventAt.Add(1500*time.Millisecond))
		third, thirdErr := state.fromSeries(signal, "ETH/EUR", series.Prices, series.Volumes, nil, true, eventAt.Add(2*time.Second))

		Convey("It should enqueue at most one forecast per interval", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(thirdErr, ShouldBeNil)
			So(first.Source, ShouldEqual, logic.SourcePrediction)
			So(second.Source, ShouldEqual, logic.SourcePrediction)
			So(third.Source, ShouldEqual, logic.SourcePrediction)
			So(len(state.pending), ShouldEqual, 2)

			viper.Set("story.prediction.interval", 0)
		})
	})

	Convey("Given a matured pending forecast", testingTB, func() {
		signal := newTestSignal(testingTB)
		signal.horizon = time.Minute
		state := signal.ensure("ETH/EUR")

		state.features[0] = 1.0
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		state.pending = append(state.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   109,
			forecast:      0.01,
			features:      append([]float64(nil), state.features...),
			movementScale: 0.01,
			regime: predictionRegime{
				source:   logic.SourceCausal,
				category: logic.CategoryEndogenousAlpha,
				ready:    true,
			},
		})
		state.realizedMagnitudeEMA = 0.01

		coefficients := state.learner.Coefficients()
		coefficients[1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		for index, price := range []float64{108, 109, 109.5, 110} {
			signal.trade.Update(krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "ETH/EUR",
					Price:     price,
					Qty:       1,
					Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
				},
			})
		}

		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategoryEndogenousAlpha,
			Confidence: 0.5,
		})

		_, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should drain feedback after settlement", func() {
			So(err, ShouldBeNil)
			So(state.drainFeedback(), ShouldNotBeNil)
			So(state.drainFeedback(), ShouldBeNil)
		})

		Convey("It should drain normalized chart settlement events", func() {
			events := state.drainChartEvents()

			So(len(events.Settlements), ShouldEqual, 1)
			So(events.Settlements[0].Forecast, ShouldBeBetween, 0.4, 0.6)
			So(events.Settlements[0].Actual, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalSettlePendingUsesForecastScale(testingTB *testing.T) {
	Convey("Given a matured forecast with a frozen movement scale", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		state.pending = append(state.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   100,
			forecast:      0.01,
			features:      append([]float64(nil), state.features...),
			movementScale: 0.01,
		})
		state.realizedMagnitudeEMA = 0.000001

		settlements, settleErr := state.settlePending(signal, eventAt, 101)
		So(settleErr, ShouldBeNil)

		Convey("It should map with the scale from forecast time", func() {
			So(len(settlements), ShouldEqual, 1)
			So(settlements[0].Forecast, ShouldEqual, 0.5)
			So(settlements[0].Actual, ShouldEqual, 0.5)
		})
	})
}

func TestSignalSettlePendingInvalidatesShiftedRegime(testingTB *testing.T) {
	Convey("Given a forecast crossing a macro regime change", testingTB, func() {
		signal := newTestSignal(testingTB)
		signal.horizon = time.Minute
		state := signal.ensure("ETH/EUR")
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategoryEndogenousAlpha,
			Confidence: 0.8,
		})
		state.enqueueForecast(signal, eventAt, 100, 0.01, 0.01)
		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategorySystemicBeta,
			Confidence: 0.9,
		})

		settlements, settleErr := state.settlePending(
			signal,
			eventAt.Add(time.Minute),
			110,
		)

		Convey("It should publish settlement but skip learner feedback", func() {
			So(settleErr, ShouldBeNil)
			So(len(settlements), ShouldEqual, 1)
			So(state.feedbackSamples, ShouldEqual, 0)
			So(state.drainFeedback(), ShouldBeNil)
		})
	})
}

func TestSignalSettlePendingInvalidatesPanicShift(testingTB *testing.T) {
	Convey("Given a calm forecast settling into panic", testingTB, func() {
		signal := newTestSignal(testingTB)
		signal.horizon = time.Minute
		state := signal.ensure("ETH/EUR")
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategoryEndogenousAlpha,
			Confidence: 0.8,
		})
		state.enqueueForecast(signal, eventAt, 100, 0.01, 0.01)
		state.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategoryLiquidityShock,
			Confidence: 0.9,
		})

		settlements, settleErr := state.settlePending(
			signal,
			eventAt.Add(time.Minute),
			110,
		)

		Convey("It should skip learner feedback on contagion panic shift", func() {
			So(settleErr, ShouldBeNil)
			So(len(settlements), ShouldEqual, 1)
			So(state.feedbackSamples, ShouldEqual, 0)
		})
	})
}

func TestSignalMovementScale(testingTB *testing.T) {
	Convey("Given a calibrated realized magnitude EMA", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.realizedMagnitudeEMA = 0.000001
		prices := []float64{100, 101, 102, 103}

		Convey("It should floor normalization on span movement", func() {
			So(state.movementScale(prices), ShouldEqual, spanReturnScale(prices))
		})
	})

	Convey("Given no settled horizon magnitude yet", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		Convey("It should withhold chart normalization", func() {
			So(state.movementScale([]float64{100, 101}), ShouldEqual, 0)
		})

		Convey("It should normalize confidence from the feature baseline", func() {
			state.features[0] = 0.5
			confidence, err := state.movementConfidence(0.02, []float64{100, 101})

			So(err, ShouldBeNil)
			So(confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalChartUsesBaselineScaleBeforeMagnitudeEMA(testingTB *testing.T) {
	Convey("Given trade measurements before the first settlement", testingTB, func() {
		signal := newTestSignal(testingTB)
		signal.horizon = time.Minute
		state := signal.ensure("ETH/EUR")

		state.features[0] = 0.5
		state.realizedMagnitudeEMA = 0.01
		coefficients := state.learner.Coefficients()
		coefficients[1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)
		series := signal.trade.Series("ETH/EUR")

		_, err := signal.Measure(measurementArtifact("ETH/EUR"))
		events := state.drainChartEvents()

		Convey("It should publish forecast using the feature baseline scale", func() {
			So(err, ShouldBeNil)
			So(events.HasForecast, ShouldBeTrue)
			So(events.ForecastTarget, ShouldEqual, float64(series.At.Add(signal.horizon).Unix()))
		})
	})
}

func TestSignalFlatTapeNormalizedForecast(testingTB *testing.T) {
	Convey("Given calibrated horizon scale and micro-tick drift", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.realizedMagnitudeEMA = 0.01
		state.features[0] = 0.5
		coefficients := state.learner.Coefficients()
		coefficients[1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		_, err := signal.Measure(measurementArtifact("ETH/EUR"))
		events := state.drainChartEvents()

		Convey("It should keep normalized forecasts in the unit band", func() {
			So(err, ShouldBeNil)
			So(events.HasForecast, ShouldBeTrue)
			So(math.Abs(events.Forecast), ShouldBeLessThan, 1)
		})
	})
}

func TestSignalMeasureSettlementPrice(testingTB *testing.T) {
	Convey("Given a matured forecast and a spike print on the tape", testingTB, func() {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.features[0] = 1.0
		state.pending = append(state.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   100,
			forecast:      0.01,
			features:      append([]float64(nil), state.features...),
			movementScale: 0.01,
		})
		state.realizedMagnitudeEMA = 0.01

		coefficients := state.learner.Coefficients()
		coefficients[1] = 0.05
		So(state.learner.SetCoefficients(coefficients), ShouldBeNil)

		updates := make(krakenmarket.TradeUpdates, 4)

		for index, price := range []float64{99.99, 100, 100, 200} {
			updates[index] = &krakenmarket.TradeUpdate{
				Symbol:    "ETH/EUR",
				Price:     price,
				Qty:       1,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			}
		}

		signal.trade.Update(updates)

		_, err := signal.Measure(measurementArtifact("ETH/EUR"))
		events := state.drainChartEvents()

		Convey("It should settle on the tape median instead of the spike", func() {
			So(err, ShouldBeNil)
			So(len(events.Settlements), ShouldEqual, 1)
			So(events.Settlements[0].Actual, ShouldEqual, 0)
		})
	})
}

func TestSignalMovementUnits(testingTB *testing.T) {
	Convey("Given an extreme raw forecast", testingTB, func() {
		Convey("It should stay inside the signed unit band", func() {
			positiveUnits, positiveErr := movementUnits(610, 0.01)
			negativeUnits, negativeErr := movementUnits(-610, 0.01)

			So(positiveErr, ShouldBeNil)
			So(negativeErr, ShouldBeNil)
			So(math.Abs(positiveUnits), ShouldBeLessThanOrEqualTo, 1)
			So(math.Abs(negativeUnits), ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMovementConfidence(testingTB *testing.T) {
	Convey("Given a realized movement scale", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.realizedMagnitudeEMA = 0.01
		prices := []float64{100, 100.01, 100.02}

		Convey("It should stay inside the unit band", func() {
			largeConfidence, largeErr := state.movementConfidence(610, prices)
			zeroConfidence, zeroErr := state.movementConfidence(0, prices)

			So(largeErr, ShouldBeNil)
			So(zeroErr, ShouldNotBeNil)
			So(largeConfidence, ShouldBeLessThanOrEqualTo, 1)
			So(zeroConfidence, ShouldEqual, 0)
		})

		Convey("It should rise with forecast intensity", func() {
			strongConfidence, strongErr := state.movementConfidence(0.02, prices)
			weakConfidence, weakErr := state.movementConfidence(0.002, prices)

			So(strongErr, ShouldBeNil)
			So(weakErr, ShouldBeNil)
			So(strongConfidence, ShouldBeGreaterThan, weakConfidence)
		})
	})
}

func TestSignalMeasureWithholdsWithoutErrorOnFlatPrices(testingTB *testing.T) {
	Convey("Given flat endpoint trades with upstream features", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		for featureIndex := range state.features {
			state.features[featureIndex] = 0.4
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		measurement, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should withhold without aborting the signal loop", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourcePrediction)
		})
	})
}

func TestSignalLearningTarget(testingTB *testing.T) {
	Convey("Given a collapsed realized magnitude scale", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		state.realizedMagnitudeEMA = 1e-8

		collapsedScale := 1e-8
		degenerateTarget := 0.01 / (1 + 0.01/collapsedScale)
		target := state.learningTarget(0.01)

		Convey("It should avoid collapsing the target to the scale floor", func() {
			So(math.Abs(target), ShouldBeGreaterThan, math.Abs(degenerateTarget)*10)
		})
	})
}

func TestSignalLearn(testingTB *testing.T) {
	Convey("Given repeated settlement residuals", testingTB, func() {
		signal := newTestSignal(testingTB)
		state := signal.ensure("ETH/EUR")

		for featureIndex := range state.features {
			state.features[featureIndex] = 0.5
		}

		state.realizedMagnitudeEMA = 0.01

		for range 512 {
			So(state.learn(signal, state.features, -0.5), ShouldBeNil)
		}

		Convey("It should keep forecasts bounded", func() {
			forecast, predictErr := state.predict(state.features)
			So(predictErr, ShouldBeNil)
			So(math.Abs(forecast), ShouldBeLessThan, 0.2)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), nil)
	state := signal.ensure("ETH/EUR")

	state.recordFeatureMeasurement(logic.Measurement{
		Source:     logic.SourcePumpDump,
		Symbol:     "ETH/EUR",
		Confidence: 0.5,
	})

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seedTrades(signal, "ETH/EUR", eventAt, 32, 100)

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(measurementArtifact("ETH/EUR"))

		if err != nil {
			b.Fatal(err)
		}
	}
}
