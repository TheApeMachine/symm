package prediction

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSignalMeasureMeasurement(t *testing.T) {
	Convey("Given upstream measurements in the ring", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.Record(logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.75,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should rebuild the feature snapshot", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(signal.features[featureSourceIndex(logic.SourcePumpDump)], ShouldEqual, 0.75)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			8,
			time.Minute,
			0.1,
			1000.0,
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
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			4,
			time.Minute,
			0.1,
			1000.0,
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
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		featureSignal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityMeasurement),
			8,
			time.Minute,
			0.1,
			1000.0,
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

		for index := range 4 {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Price:  100 + float64(index),
				Qty:    1,
			})
		}

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should publish a unit-band forward confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePrediction)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
			So(measurement.Elapsed, ShouldEqual, time.Minute.Seconds())
		})
	})

	Convey("Given a matured pending forecast", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
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

		for _, price := range []float64{108, 109, 109.5, 110} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Price:  price,
				Qty:    1,
			})
		}

		_, err := signal.Measure(nil, eventAt)

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
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "ETH/EUR"})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a ticker entity signal", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTick),
			8,
			time.Minute,
			0.1,
			1000.0,
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
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
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

func TestSignalMovementScale(t *testing.T) {
	Convey("Given a calibrated realized magnitude EMA", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.realizedMagnitudeEMA = 0.000001
		prices := []float64{100, 101, 102, 103}

		Convey("It should floor normalization on span movement", func() {
			So(signal.movementScale(prices), ShouldEqual, spanReturnScale(prices))
		})
	})

	Convey("Given no settled horizon magnitude yet", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		Convey("It should withhold chart normalization", func() {
			So(signal.movementScale([]float64{100, 101}), ShouldEqual, 0)
		})
	})
}

func TestSignalChartWaitsForMagnitudeEMA(t *testing.T) {
	Convey("Given trade measurements before the first settlement", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.features[0] = 0.5

		for index := range 4 {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Price:  100 + float64(index)*0.0000001,
				Qty:    1,
			})
		}

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		events := signal.DrainChartEvents()

		Convey("It should not publish chart frames yet", func() {
			So(err, ShouldBeNil)
			So(events.HasForecast, ShouldBeFalse)
		})
	})
}

func TestSignalFlatTapeNormalizedForecast(t *testing.T) {
	Convey("Given calibrated horizon scale and micro-tick drift", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.realizedMagnitudeEMA = 0.01
		signal.features[0] = 0.5

		for index := range 4 {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Price:  100 + float64(index)*0.0000001,
				Qty:    1,
			})
		}

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
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

		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.pending = append(signal.pending, &pendingForecast{
			matureAt:      eventAt.Add(-time.Second),
			anchorPrice:   100,
			forecast:      0.01,
			features:      append([]float64(nil), signal.features...),
			movementScale: 0.01,
		})
		signal.realizedMagnitudeEMA = 0.01

		for _, price := range []float64{100, 100, 100, 100, 1_000_000} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Price:  price,
				Qty:    1,
			})
		}

		_, err := signal.Measure(nil, eventAt)
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
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.realizedMagnitudeEMA = 0.01

		Convey("It should stay inside the signed unit band", func() {
			So(math.Abs(signal.movementUnits(610, 0.01)), ShouldBeLessThanOrEqualTo, 1)
			So(math.Abs(signal.movementUnits(-610, 0.01)), ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMovementConfidence(t *testing.T) {
	Convey("Given a realized movement scale", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
		)

		signal.realizedMagnitudeEMA = 0.01

		prices := []float64{100, 100.01, 100.02}

		Convey("It should stay inside the unit band", func() {
			So(signal.movementConfidence(610, prices), ShouldBeLessThanOrEqualTo, 1)
			So(signal.movementConfidence(0, prices), ShouldBeLessThan, 0.35)
		})

		Convey("It should rise with forecast intensity", func() {
			So(
				signal.movementConfidence(0.02, prices),
				ShouldBeGreaterThan,
				signal.movementConfidence(0.002, prices),
			)
		})
	})
}

func TestSignalLearn(t *testing.T) {
	Convey("Given repeated settlement residuals", t, func() {
		signal, _ := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			time.Minute,
			0.1,
			1000.0,
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
	signal, _ := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityTrade),
		8,
		time.Minute,
		0.1,
		1000.0,
	)

	featureSignal, _ := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityMeasurement),
		8,
		time.Minute,
		0.1,
		1000.0,
	)

	featureSignal.Record(logic.Measurement{
		Source:     logic.SourcePumpDump,
		Symbol:     "ETH/EUR",
		Confidence: 0.5,
	})
	signal.ApplyFeatures(featureSignal.Features())

	for index := range 32 {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "ETH/EUR",
			Price:  100 + float64(index),
			Qty:    float64(index%5) + 1,
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
