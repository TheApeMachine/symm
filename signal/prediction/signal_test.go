package prediction

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func init() {
	viper.Set("signals.prediction.measurements_capacity", 4)
	viper.Set("story.prediction.horizon", time.Minute)
	viper.Set("story.prediction.interval", 0)
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	for index := range count {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		})
	}
}

func TestSignalMeasureMeasurement(t *testing.T) {
	Convey("Given upstream measurements in the ring", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			nil,
		)

		signal.Record(logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.75,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should rebuild the feature snapshot", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "")
			So(signal.features[featureSourceIndex(logic.SourcePumpDump)], ShouldEqual, 0.75)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			nil,
		)

		signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 100, Qty: 1})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSignalRecord(t *testing.T) {
	Convey("Given a new signal", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 100, Qty: 1}), ShouldBeTrue)
		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 101, Qty: 1}), ShouldBeTrue)

		Convey("It should count down warmup without scanning the ring", func() {
			So(signal.warmupRemaining, ShouldEqual, 2)
			So(signal.WarmupFilled(), ShouldEqual, 2)
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given source confidences and trade prices", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		featureSignal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			nil,
		)

		featureSignal.Record(logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.8,
		})
		featureSignal.Record(logic.Measurement{
			Source:     logic.SourceHawkes,
			Symbol:     "ETH/EUR",
			Confidence: 0.2,
		})

		_, featureErr := featureSignal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		So(featureErr, ShouldBeNil)

		signal.ApplyFeatures(featureSignal.Features())
		signal.realizedMagnitudeEMA = 0.01
		coefficients := signal.learner.Coefficients()
		coefficients[featureSourceIndex(logic.SourcePumpDump)+1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		measurement, err := signal.Measure(nil, eventAt.Add(time.Second))

		Convey("It should publish a unit-band forward confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePrediction)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
			So(measurement.Elapsed, ShouldEqual, time.Minute.Seconds())
		})
	})

	Convey("Given the configured forecast interval", t, func() {
		viper.Set("story.prediction.interval", time.Second)

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.ApplyFeatures([]float64{0.8, 0.2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		signal.realizedMagnitudeEMA = 0.01
		coefficients := signal.learner.Coefficients()
		coefficients[1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		first, firstErr := signal.Measure(nil, eventAt.Add(time.Second))
		second, secondErr := signal.Measure(nil, eventAt.Add(1500*time.Millisecond))
		third, thirdErr := signal.Measure(nil, eventAt.Add(2*time.Second))

		Convey("It should enqueue at most one forecast per interval", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(thirdErr, ShouldBeNil)
			So(first.Source, ShouldEqual, logic.SourcePrediction)
			So(second.Source, ShouldEqual, logic.SourcePrediction)
			So(third.Source, ShouldEqual, logic.SourcePrediction)
			So(len(signal.pending), ShouldEqual, 2)

			viper.Set("story.prediction.interval", 0)
		})
	})

	Convey("Given a matured pending forecast", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.features[0] = 1.0
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.pending = append(signal.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   109,
			forecast:      0.01,
			features:      append([]float64(nil), signal.features...),
			movementScale: 0.01,
		})
		signal.realizedMagnitudeEMA = 0.01

		signal.realizedMagnitudeEMA = 0.01
		coefficients := signal.learner.Coefficients()
		coefficients[1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		for index, price := range []float64{108, 109, 109.5, 110} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "ETH/EUR",
				Price:     price,
				Qty:       1,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		_, err := signal.Measure(nil, eventAt.Add(time.Second))

		Convey("It should drain feedback after settlement", func() {
			So(err, ShouldBeNil)
			So(signal.DrainFeedback(), ShouldNotBeNil)
			So(signal.DrainFeedback(), ShouldBeNil)
		})

		Convey("It should drain normalized chart settlement events", func() {
			events := signal.DrainChartEvents()

			So(len(events.Settlements), ShouldEqual, 1)
			So(events.Settlements[0].Forecast, ShouldBeBetween, 0.4, 0.6)
			So(events.Settlements[0].Actual, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "ETH/EUR"})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a ticker entity signal", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTick),
			nil,
		)

		signal.Record(&krakenmarket.TickerUpdate{
			Symbol: "ETH/EUR",
			Bid:    100,
			Ask:    101,
			BidQty: 1,
			AskQty: 1,
		})

		events := signal.DrainChartEvents()

		Convey("It should not produce chart events", func() {
			So(events.HasForecast, ShouldBeFalse)
			So(len(events.Settlements), ShouldEqual, 0)
		})
	})
}

func TestSignalSettlePendingUsesForecastScale(t *testing.T) {
	Convey("Given a matured forecast with a frozen movement scale", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.pending = append(signal.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   100,
			forecast:      0.01,
			features:      append([]float64(nil), signal.features...),
			movementScale: 0.01,
		})
		signal.realizedMagnitudeEMA = 0.000001

		settlements, settleErr := signal.settlePending(eventAt, 101)
		So(settleErr, ShouldBeNil)

		Convey("It should map with the scale from forecast time", func() {
			So(len(settlements), ShouldEqual, 1)
			So(settlements[0].Forecast, ShouldEqual, 0.5)
			So(settlements[0].Actual, ShouldEqual, 0.5)
		})
	})
}

func TestSignalSettlePendingInvalidatesShiftedRegime(t *testing.T) {
	Convey("Given a forecast crossing a macro regime change", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategoryEndogenousAlpha,
			Confidence: 0.8,
		})
		signal.enqueueForecast(eventAt, 100, 0.01, 0.01)
		signal.recordFeatureMeasurement(logic.Measurement{
			Source:     logic.SourceCausal,
			Category:   logic.CategorySystemicBeta,
			Confidence: 0.9,
		})

		settlements, settleErr := signal.settlePending(
			eventAt.Add(time.Minute),
			110,
		)

		Convey("It should publish settlement but skip learner feedback", func() {
			So(settleErr, ShouldBeNil)
			So(len(settlements), ShouldEqual, 1)
			So(signal.feedbackSamples, ShouldEqual, 0)
			So(signal.DrainFeedback(), ShouldBeNil)
		})
	})
}

func TestSignalMovementScale(t *testing.T) {
	Convey("Given a calibrated realized magnitude EMA", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.realizedMagnitudeEMA = 0.000001
		prices := []float64{100, 101, 102, 103}

		Convey("It should floor normalization on span movement", func() {
			So(signal.movementScale(prices), ShouldEqual, spanReturnScale(prices))
		})
	})

	Convey("Given no settled horizon magnitude yet", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		Convey("It should withhold chart normalization", func() {
			So(signal.movementScale([]float64{100, 101}), ShouldEqual, 0)
		})

		Convey("It should normalize confidence from the feature baseline", func() {
			signal.features[0] = 0.5
			confidence, err := signal.movementConfidence(0.02, []float64{100, 101})

			So(err, ShouldBeNil)
			So(confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalChartUsesBaselineScaleBeforeMagnitudeEMA(t *testing.T) {
	Convey("Given trade measurements before the first settlement", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.features[0] = 0.5
		signal.realizedMagnitudeEMA = 0.01
		coefficients := signal.learner.Coefficients()
		coefficients[1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		measureAt := eventAt.Add(time.Second)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		_, err := signal.Measure(nil, measureAt)
		events := signal.DrainChartEvents()

		Convey("It should publish forecast using the feature baseline scale", func() {
			So(err, ShouldBeNil)
			So(events.HasForecast, ShouldBeTrue)
			So(events.ForecastTarget, ShouldEqual, float64(measureAt.Add(time.Minute).Unix()))
		})
	})
}

func TestSignalFlatTapeNormalizedForecast(t *testing.T) {
	Convey("Given calibrated horizon scale and micro-tick drift", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.realizedMagnitudeEMA = 0.01
		signal.features[0] = 0.5
		coefficients := signal.learner.Coefficients()
		coefficients[1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		measureAt := eventAt.Add(time.Second)
		seedTrades(signal, "ETH/EUR", eventAt, 4, 100)

		_, err := signal.Measure(nil, measureAt)
		events := signal.DrainChartEvents()

		Convey("It should keep normalized forecasts in the unit band", func() {
			So(err, ShouldBeNil)
			So(events.HasForecast, ShouldBeTrue)
			So(math.Abs(events.Forecast), ShouldBeLessThan, 1)
		})
	})
}

func TestSignalMeasureSettlementPrice(t *testing.T) {
	Convey("Given a matured forecast and a spike print on the tape", t, func() {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.features[0] = 1.0
		signal.pending = append(signal.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   100,
			forecast:      0.01,
			features:      append([]float64(nil), signal.features...),
			movementScale: 0.01,
		})
		signal.realizedMagnitudeEMA = 0.01

		coefficients := signal.learner.Coefficients()
		coefficients[1] = 0.05
		So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

		for index, price := range []float64{99.99, 100, 100, 200} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "ETH/EUR",
				Price:     price,
				Qty:       1,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		_, err := signal.Measure(nil, eventAt.Add(time.Second))
		events := signal.DrainChartEvents()

		Convey("It should settle on the tape median instead of the spike", func() {
			So(err, ShouldBeNil)
			So(len(events.Settlements), ShouldEqual, 1)
			So(events.Settlements[0].Actual, ShouldEqual, 0)
		})
	})
}

func TestSignalMovementUnits(t *testing.T) {
	Convey("Given an extreme raw forecast", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.realizedMagnitudeEMA = 0.01

		Convey("It should stay inside the signed unit band", func() {
			positiveUnits, positiveErr := signal.movementUnits(610, 0.01)
			negativeUnits, negativeErr := signal.movementUnits(-610, 0.01)

			So(positiveErr, ShouldBeNil)
			So(negativeErr, ShouldBeNil)
			So(math.Abs(positiveUnits), ShouldBeLessThanOrEqualTo, 1)
			So(math.Abs(negativeUnits), ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMovementConfidence(t *testing.T) {
	Convey("Given a realized movement scale", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.realizedMagnitudeEMA = 0.01

		prices := []float64{100, 100.01, 100.02}

		Convey("It should stay inside the unit band", func() {
			largeConfidence, largeErr := signal.movementConfidence(610, prices)
			zeroConfidence, zeroErr := signal.movementConfidence(0, prices)

			So(largeErr, ShouldBeNil)
			So(zeroErr, ShouldNotBeNil)
			So(largeConfidence, ShouldBeLessThanOrEqualTo, 1)
			So(zeroConfidence, ShouldEqual, 0)
		})

		Convey("It should rise with forecast intensity", func() {
			strongConfidence, strongErr := signal.movementConfidence(0.02, prices)
			weakConfidence, weakErr := signal.movementConfidence(0.002, prices)

			So(strongErr, ShouldBeNil)
			So(weakErr, ShouldBeNil)
			So(strongConfidence, ShouldBeGreaterThan, weakConfidence)
		})
	})
}

func TestSignalMeasureWithholdsWithoutErrorOnFlatPrices(t *testing.T) {
	Convey("Given flat endpoint trades with upstream features", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		for featureIndex := range signal.features {
			signal.features[featureIndex] = 0.4
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index := range 4 {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "ETH/EUR",
				Price:     100,
				Qty:       1,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		measurement, err := signal.Measure(nil, eventAt.Add(time.Second))

		Convey("It should withhold without aborting the signal loop", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourcePrediction)
		})
	})
}

func TestSignalLearningTarget(t *testing.T) {
	Convey("Given a collapsed realized magnitude scale", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		signal.realizedMagnitudeEMA = 1e-8

		collapsedScale := 1e-8
		degenerateTarget := 0.01 / (1 + 0.01/collapsedScale)
		target := signal.learningTarget(0.01)

		Convey("It should avoid collapsing the target to the scale floor", func() {
			So(math.Abs(target), ShouldBeGreaterThan, math.Abs(degenerateTarget)*10)
		})
	})
}

func TestSignalLearn(t *testing.T) {
	Convey("Given repeated settlement residuals", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			nil,
		)

		for featureIndex := range signal.features {
			signal.features[featureIndex] = 0.5
		}

		signal.realizedMagnitudeEMA = 0.01

		for range 512 {
			So(signal.learn(signal.features, -0.5), ShouldBeNil)
		}

		Convey("It should keep forecasts bounded", func() {
			forecast, predictErr := signal.predict(signal.features)
			So(predictErr, ShouldBeNil)
			So(math.Abs(forecast), ShouldBeLessThan, 0.2)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityTrade),
		nil,
	)

	featureSignal := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityMeasurement),
		nil,
	)

	featureSignal.Record(logic.Measurement{
		Source:     logic.SourcePumpDump,
		Symbol:     "ETH/EUR",
		Confidence: 0.5,
	})
	signal.ApplyFeatures(featureSignal.Features())

	for index := range 32 {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "ETH/EUR",
			Price:     100 + float64(index),
			Qty:       float64(index%5) + 1,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Millisecond),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		if err != nil {
			b.Fatal(err)
		}
	}
}
