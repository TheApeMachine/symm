package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var hawkesCategories = []logic.CategoryType{
	logic.CategoryFrenzy,
	logic.CategorySaturation,
	logic.CategoryOrganic,
	logic.CategoryExhaustion,
}

func TestSignalIngestRoles(testingTB *testing.T) {
	Convey("Given a Hawkes signal", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		Convey("When ingest roles are requested", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"trade"})
		})

		Convey("It ignores non-trade rows", func() {
			tickerMeasurements, tickerErr := signal.Measure(market.Input{Role: "ticker"}, nil)
			bookMeasurements, bookErr := signal.Measure(market.Input{Role: "book"}, nil)

			So(tickerErr, ShouldBeNil)
			So(bookErr, ShouldBeNil)
			So(tickerMeasurements, ShouldHaveLength, 0)
			So(bookMeasurements, ShouldHaveLength, 0)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a Hawkes signal", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		measurements, err := signal.Measure(tradeInput(
			trades("MATIC/USD", 128, startAt(0), 100*time.Millisecond),
		), nil)

		Convey("It emits typed Hawkes measurements after the excitation pipeline warms", func() {
			So(err, ShouldBeNil)
			So(len(measurements), ShouldBeGreaterThan, 0)

			for _, measurement := range measurements {
				assertMeasurement(measurement, "MATIC/USD")
			}
		})
	})
}

func TestTradeMeasure(testingTB *testing.T) {
	Convey("Given a Hawkes trade role", testingTB, func() {
		role := NewTrade()
		var result *logic.Measurement

		for _, row := range trades("BTC/USD", 160, startAt(0), 100*time.Millisecond) {
			measurement, err := role.Measure(row)
			So(err, ShouldBeNil)

			if measurement != nil {
				result = measurement
			}
		}

		Convey("When enough trade arrivals have warmed the excitation pipeline", func() {
			So(result, ShouldNotBeNil)
			assertMeasurement(result, "BTC/USD")
			So(result.Metric("frenzy"), ShouldBeGreaterThanOrEqualTo, 0)
			So(result.Metric("saturation"), ShouldBeGreaterThanOrEqualTo, 0)
			So(result.Metric("organic"), ShouldBeGreaterThanOrEqualTo, 0)
			So(result.Metric("exhaustion"), ShouldBeGreaterThanOrEqualTo, 0)
			So(result.Metric("branchingRatio"), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	rows := trades("MATIC/USD", 128, startAt(0), 100*time.Millisecond)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal := NewSignal(context.Background())
		_, _ = signal.Measure(tradeInput(rows), nil)
		_ = signal.Close()
	}
}

func assertMeasurement(measurement *logic.Measurement, symbol string) {
	So(measurement.Source, ShouldEqual, logic.SourceHawkes)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Metric("value"), ShouldBeGreaterThan, 0)
	So(measurement.Confidence, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(measurement.HasDistribution(), ShouldBeTrue)
	So(hawkesCategory(measurement.DominantCategory()), ShouldBeTrue)
}

func tradeInput(rows kraken.TradeDataSlice) market.Input {
	return market.Input{
		Role:  "trade",
		Trade: rows,
	}
}

func trades(
	symbol string,
	count int,
	start time.Time,
	step time.Duration,
) kraken.TradeDataSlice {
	rows := make(kraken.TradeDataSlice, 0, count)

	for index := range count {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		rows = append(rows, kraken.TradeData{
			Symbol:    symbol,
			Side:      side,
			Price:     100,
			Qty:       1,
			Timestamp: start.Add(time.Duration(index) * step),
		})
	}

	return rows
}

func startAt(offset int) time.Time {
	return time.Date(2026, 5, 30, 12, 0, offset, 0, time.UTC)
}

func hawkesCategory(category logic.CategoryType) bool {
	for _, candidate := range hawkesCategories {
		if category == candidate {
			return true
		}
	}

	return false
}
