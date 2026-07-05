package cvd

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a CVD signal", t, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		Convey("When ingest roles are requested", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"trade"})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a CVD signal", t, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		type ignoredCase struct {
			name  string
			input market.Input
		}

		cases := []ignoredCase{
			{name: "ticker rows", input: market.Input{Role: "ticker"}},
			{name: "book rows", input: market.Input{Role: "book"}},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				measurements, err := signal.Measure(testCase.input, nil)

				Convey("Then no CVD measurements should be emitted", func() {
					So(err, ShouldBeNil)
					So(measurements, ShouldHaveLength, 0)
				})
			})
		}

		Convey("When measuring trade rows", func() {
			measurements, err := signal.Measure(tradeInput(
				trades("MATIC/USD", "buy", 100, 1, 30, time.Now().UTC()),
			), nil)

			Convey("Then CVD measurements should be emitted", func() {
				So(err, ShouldBeNil)
				So(len(measurements), ShouldBeGreaterThan, 0)

				for _, measurement := range measurements {
					assertMeasurement(measurement, "MATIC/USD")
				}
			})
		})
	})
}

func TestTradeMeasure(t *testing.T) {
	Convey("Given a CVD trade role", t, func() {
		type tradeCase struct {
			name         string
			rows         kraken.TradeDataSlice
			wantCategory logic.CategoryType
		}

		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		cases := []tradeCase{
			{
				name: "hidden absorption",
				rows: kraken.TradeDataSlice{
					tradeRow("BTC/USD", "buy", 100, 10, start),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(4*time.Second)),
				},
				wantCategory: logic.CategoryHiddenAbsorption,
			},
			{
				name: "aggressive drive",
				rows: kraken.TradeDataSlice{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "buy", 101, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 102, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 103, 2, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 104, 2, start.Add(4*time.Second)),
				},
				wantCategory: logic.CategoryAggressiveDrive,
			},
			{
				name: "stochastic balance",
				rows: kraken.TradeDataSlice{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "sell", 100, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100.1, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "sell", 100.1, 2, start.Add(3*time.Second)),
				},
				wantCategory: logic.CategoryStochasticBalance,
			},
			{
				name: "volume starvation",
				rows: kraken.TradeDataSlice{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "sell", 100, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100.01, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "sell", 100.01, 2, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 100.02, 2, start.Add(4*time.Second)),
					tradeRow("BTC/USD", "sell", 100.02, 2, start.Add(5*time.Second)),
					tradeRow("BTC/USD", "buy", 100.03, 2, start.Add(6*time.Second)),
					tradeRow("BTC/USD", "sell", 100.03, 2, start.Add(7*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(8*time.Second)),
					tradeRow("BTC/USD", "sell", 100.04, 0.001, start.Add(9*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(10*time.Second)),
					tradeRow("BTC/USD", "sell", 100.04, 0.001, start.Add(11*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(12*time.Second)),
				},
				wantCategory: logic.CategoryVolumeStarvation,
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				role := NewTrade()
				var result *logic.Measurement

				for _, row := range testCase.rows {
					measurement, err := role.Measure(row)
					So(err, ShouldBeNil)

					if measurement != nil {
						result = measurement
					}
				}

				Convey(fmt.Sprintf("Then CVD should classify %s", testCase.name), func() {
					So(result, ShouldNotBeNil)
					So(result.Source, ShouldEqual, logic.SourceCVD)
					So(result.Symbol, ShouldEqual, "BTC/USD")
					So(result.DominantCategory(), ShouldEqual, testCase.wantCategory)
					So(result.Confidence, ShouldBeGreaterThan, 0)
				})
			})
		}
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	rows := trades("MATIC/USD", "buy", 100, 1, 8, time.Now().UTC())

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal := NewSignal(context.Background())
		_, _ = signal.Measure(tradeInput(rows), nil)
		_ = signal.Close()
	}
}

func assertMeasurement(measurement *logic.Measurement, symbol string) {
	So(measurement.Source, ShouldEqual, logic.SourceCVD)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Metric("strength"), ShouldBeGreaterThanOrEqualTo, 0)
	So(measurement.Confidence, ShouldBeGreaterThan, 0)
	So(cvdCategory(measurement.DominantCategory()), ShouldBeTrue)
}

func tradeInput(rows kraken.TradeDataSlice) market.Input {
	return market.Input{
		Role:  "trade",
		Trade: rows,
	}
}

func trades(
	symbol string,
	side string,
	price float64,
	quantity float64,
	count int,
	start time.Time,
) kraken.TradeDataSlice {
	rows := make(kraken.TradeDataSlice, 0, count)

	for index := range count {
		rows = append(rows, tradeRow(
			symbol,
			side,
			price+float64(index)*0.01,
			quantity,
			start.Add(time.Duration(index)*time.Second),
		))
	}

	return rows
}

func tradeRow(
	symbol string,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       quantity,
		Timestamp: at,
	}
}

func cvdCategory(category logic.CategoryType) bool {
	switch category {
	case logic.CategoryHiddenAbsorption,
		logic.CategoryAggressiveDrive,
		logic.CategoryStochasticBalance,
		logic.CategoryVolumeStarvation:
		return true
	default:
		return false
	}
}
