package cvd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a CVD signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer signal.Close()

		Convey("When ingest roles are requested", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"trade"})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a CVD signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer signal.Close()

		type ignoredCase struct {
			name  string
			input any
		}

		cases := []ignoredCase{
			{name: "ticker rows", input: kraken.TickerData{}},
			{name: "book rows", input: kraken.BookData{}},
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
			measurements := make([]*types.Measurement, 0)

			for _, row := range trades("MATIC/USD", "buy", 100, 1, 30, time.Now().UTC()) {
				out, err := signal.Measure(row, nil)
				So(err, ShouldBeNil)
				measurements = append(measurements, out...)
			}

			Convey("Then CVD measurements should be emitted", func() {
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
			rows         []kraken.TradeData
			wantCategory types.CategoryType
		}

		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		cases := []tradeCase{
			{
				name: "hidden absorption",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 10, start),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(4*time.Second)),
				},
				wantCategory: types.CategoryHiddenAbsorption,
			},
			{
				name: "aggressive drive",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "buy", 101, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 102, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 103, 2, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 104, 2, start.Add(4*time.Second)),
				},
				wantCategory: types.CategoryAggressiveDrive,
			},
			{
				name: "stochastic balance",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "sell", 100, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100.1, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "sell", 100.1, 2, start.Add(3*time.Second)),
				},
				wantCategory: types.CategoryStochasticBalance,
			},
			{
				name: "volume starvation",
				rows: []kraken.TradeData{
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
				wantCategory: types.CategoryVolumeStarvation,
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				role := NewTrade()
				var result *types.Measurement

				for _, row := range testCase.rows {
					measurements, err := role.Measure(row)
					So(err, ShouldBeNil)

					if len(measurements) > 0 {
						result = measurements[len(measurements)-1]
					}
				}

				Convey(fmt.Sprintf("Then CVD should classify %s", testCase.name), func() {
					So(result, ShouldNotBeNil)
					So(result.Source, ShouldEqual, types.SourceCVD)
					So(result.Symbol, ShouldEqual, "BTC/USD")

					category := types.CategoryTypeNone
					confidence := 0.0

					for _, categoryRow := range result.Categories {
						if categoryRow.Confidence <= confidence {
							continue
						}

						category = categoryRow.Type
						confidence = categoryRow.Confidence
					}

					So(category, ShouldEqual, testCase.wantCategory)
					So(confidence, ShouldBeGreaterThan, 0)
				})
			})
		}
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	rows := trades("MATIC/USD", "buy", 100, 1, 8, time.Now().UTC())

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal[any](context.Background())

		for _, row := range rows {
			_, _ = signal.Measure(row, nil)
		}

		_ = signal.Close()
	}
}

func assertMeasurement(measurement *types.Measurement, symbol string) {
	So(measurement.Source, ShouldEqual, types.SourceCVD)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Metrics["strength"], ShouldBeGreaterThanOrEqualTo, 0)

	category := types.CategoryTypeNone
	confidence := 0.0

	for _, categoryRow := range measurement.Categories {
		if categoryRow.Confidence <= confidence {
			continue
		}

		category = categoryRow.Type
		confidence = categoryRow.Confidence
	}

	So(confidence, ShouldBeGreaterThan, 0)
	So(cvdCategory(category), ShouldBeTrue)
}

func trades(
	symbol string,
	side string,
	price float64,
	quantity float64,
	count int,
	start time.Time,
) []kraken.TradeData {
	rows := make([]kraken.TradeData, 0, count)

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
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}

func cvdCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryHiddenAbsorption,
		types.CategoryAggressiveDrive,
		types.CategoryStochasticBalance,
		types.CategoryVolumeStarvation:
		return true
	default:
		return false
	}
}
