package hawkes

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestTradeMeasure(t *testing.T) {
	Convey("Given the first precisely timestamped trade arrival", t, func() {
		trade := NewTrade()
		at := time.Date(2026, 7, 12, 2, 0, 0, 17, time.UTC)
		measurements, err := trade.Measure(hawkesTrade("BTC/USD", "buy", at, 1))

		Convey("It should publish count evidence without inventing an arrival rate", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 3)
			So(measurements[0].Metric, ShouldEqual, types.MetricEventCount)
			So(measurements[0].Raw, ShouldEqual, 1.0)
			So(measurements[0].At.UnixNano(), ShouldEqual, at.UnixNano())
			So(measurements[0].Validity.Readiness, ShouldEqual,
				types.ReadinessObservation)
			So(measurements[0].Normalized.Available, ShouldBeFalse)
			So(measurements[0].Uncertainty.Available, ShouldBeFalse)
		})
	})

	Convey("Given two trades defining a positive observation interval", t, func() {
		trade := NewTrade()
		start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
		_, _ = trade.Measure(hawkesTrade("BTC/USD", "buy", start, 1))
		measurements, err := trade.Measure(hawkesTrade(
			"BTC/USD", "sell", start.Add(time.Second), 2,
		))

		Convey("It should publish empirical rates but no Hawkes baseline", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 5)
			So(countMetric(measurements, types.MetricArrivalRate), ShouldEqual, 2)
			So(countMetric(measurements, types.MetricBaselineIntensity), ShouldEqual, 0)
			So(countMetric(measurements, types.MetricConditionalIntensity), ShouldEqual, 0)
		})
	})

	Convey("Given enough alternating arrivals to identify the model", t, func() {
		trade := NewTrade()
		start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
		var measurements []*types.Measurement
		var modelMeasurements []*types.Measurement
		var projectedMeasurements []*types.Measurement
		var err error

		for index := range 32 {
			side := "buy"

			if index%2 == 1 {
				side = "sell"
			}

			measurements, err = trade.Measure(hawkesTrade(
				"BTC/USD",
				side,
				start.Add(time.Duration(index)*time.Millisecond),
				int64(index+1),
			))

			if countMetric(measurements, types.MetricExcitationAmplitude) == 4 {
				if modelMeasurements == nil {
					modelMeasurements = measurements
				}

				continue
			}

			if modelMeasurements != nil && projectedMeasurements == nil &&
				countMetric(measurements, types.MetricConditionalIntensity) == 2 {
				projectedMeasurements = measurements
			}
		}

		Convey("It should emit dimensional model evidence without strategy fields", func() {
			So(err, ShouldBeNil)
			So(modelMeasurements, ShouldNotBeNil)
			So(countMetric(modelMeasurements, types.MetricExcitationAmplitude), ShouldEqual, 4)
			So(countMetric(modelMeasurements, types.MetricImmediateOffspring), ShouldEqual, 2)
			So(countMetric(modelMeasurements, types.MetricTotalDescendants), ShouldEqual, 2)
			So(countMetric(modelMeasurements, types.MetricHawkesPoissonDelta), ShouldEqual, 1)
			So(countMetric(modelMeasurements, types.MetricCrossSelfDelta), ShouldEqual, 1)

			for _, measurement := range modelMeasurements {
				So(measurement.Source, ShouldEqual, types.SourceHawkes)
				So(measurement.Symbol, ShouldEqual, "BTC/USD")
				So(measurement.Typed(), ShouldBeTrue)
				So(measurement.Categories, ShouldBeEmpty)
				So(measurement.Metrics, ShouldBeNil)
				So(measurement.EntryBaseline, ShouldEqual, 0.0)
				So(measurement.ExitBaseline, ShouldEqual, 0.0)
			}

			So(projectedMeasurements, ShouldNotBeNil)
			So(countMetric(projectedMeasurements,
				types.MetricConditionalIntensity), ShouldEqual, 2)
			So(countMetric(projectedMeasurements,
				types.MetricExcitationAmplitude), ShouldEqual, 0)
		})
	})
}

func countMetric(
	measurements []*types.Measurement,
	metric types.MetricType,
) int {
	count := 0

	for _, measurement := range measurements {
		if measurement.Metric == metric {
			count++
		}
	}

	return count
}

func hawkesTrade(
	symbol string,
	side string,
	at time.Time,
	tradeID int64,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(float64(tradeID)),
		Qty:       1,
		OrderType: "market",
		TradeID:   tradeID,
		Timestamp: at,
	}
}

func BenchmarkTrade_Measure(t *testing.B) {
	const eventCount = 32

	rows := make([]kraken.TradeData, eventCount)
	start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)

	for index := range rows {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		rows[index] = hawkesTrade(
			"BTC/USD",
			side,
			start.Add(time.Duration(index)*time.Millisecond),
			int64(index+1),
		)
	}

	t.ReportAllocs()
	t.ResetTimer()

	for t.Loop() {
		trade := NewTrade()

		for _, row := range rows {
			measurements, err := trade.Measure(row)

			if err != nil {
				t.Fatal(err)
			}

			_ = measurements
		}
	}
}

func BenchmarkTrade_MeasureObservation(t *testing.B) {
	row := hawkesTrade(
		"BTC/USD",
		"buy",
		time.Date(2026, 7, 12, 2, 0, 0, 1, time.UTC),
		1,
	)

	t.ReportAllocs()
	t.ResetTimer()

	for t.Loop() {
		measurements, err := NewTrade().Measure(row)

		if err != nil {
			t.Fatal(err)
		}

		_ = measurements
	}
}

func BenchmarkTrade_MeasureFitted(t *testing.B) {
	trade := NewTrade()
	start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)

	for index := range 32 {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		_, err := trade.Measure(hawkesTrade(
			"BTC/USD",
			side,
			start.Add(time.Duration(index)*time.Millisecond),
			int64(index+1),
		))

		if err != nil {
			t.Fatal(err)
		}
	}

	iteration := 32
	t.ReportAllocs()
	t.ResetTimer()

	for t.Loop() {
		side := "buy"

		if iteration%2 == 1 {
			side = "sell"
		}

		measurements, err := trade.Measure(hawkesTrade(
			"BTC/USD",
			side,
			start.Add(time.Duration(iteration)*time.Millisecond),
			int64(iteration+1),
		))

		if err != nil {
			t.Fatal(err)
		}

		_ = measurements
		iteration++
	}
}
